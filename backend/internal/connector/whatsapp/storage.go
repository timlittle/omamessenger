package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// mediaStore persists the mediaRef of a message's photo, video or file, so
// a later wave's MediaFetcher can download it. WhatsApp's end-to-end
// messages carry no server-side copy to re-fetch a reference from later,
// unlike Telegram, so whatever reference a message arrives with must be
// captured now. It lives in its own database beside the account's
// whatsmeow session rather than inside it, since that session's schema is
// whatsmeow's own to manage.
type mediaStore struct {
	db *sql.DB
}

// mediaStoreSchema creates the table mediaStore reads and writes, if it
// does not already exist.
const mediaStoreSchema = `CREATE TABLE IF NOT EXISTS media_refs (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	direct_path     TEXT NOT NULL,
	media_key       BLOB NOT NULL,
	file_sha256     BLOB NOT NULL,
	file_enc_sha256 BLOB NOT NULL,
	file_length     INTEGER NOT NULL,
	mimetype        TEXT NOT NULL,
	PRIMARY KEY (conversation_id, message_id)
)`

// openMediaStore opens, creating if needed, the database that holds
// account's media references in dir.
func openMediaStore(ctx context.Context, dir, accountID string) (*mediaStore, error) {
	path := mediaStorePath(dir, accountID)
	if err := createPrivateFile(path); err != nil {
		return nil, fmt.Errorf("whatsapp: open media store: %w", err)
	}

	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: open media store: %w", err)
	}

	if _, err := db.ExecContext(ctx, mediaStoreSchema); err != nil {
		_ = db.Close() // the database could not be prepared; nothing to flush
		return nil, fmt.Errorf("whatsapp: prepare media store: %w", err)
	}

	return &mediaStore{db: db}, nil
}

// put saves ref for the message messageRemoteID of conversationRemoteID,
// replacing whatever was saved for it before.
func (m *mediaStore) put(ctx context.Context, conversationRemoteID, messageRemoteID string, ref mediaRef) error {
	const q = `INSERT INTO media_refs
		(conversation_id, message_id, direct_path, media_key, file_sha256, file_enc_sha256, file_length, mimetype)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET
			direct_path = excluded.direct_path, media_key = excluded.media_key,
			file_sha256 = excluded.file_sha256, file_enc_sha256 = excluded.file_enc_sha256,
			file_length = excluded.file_length, mimetype = excluded.mimetype`

	_, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID,
		ref.DirectPath, ref.MediaKey, ref.FileSHA256, ref.FileEncSHA256, ref.FileLength, ref.Mimetype)
	if err != nil {
		return fmt.Errorf("whatsapp: save media reference: %w", err)
	}

	return nil
}

// get returns the media reference saved for a message, or false when none
// was saved for it.
func (m *mediaStore) get(ctx context.Context, conversationRemoteID, messageRemoteID string) (mediaRef, bool, error) {
	const q = `SELECT direct_path, media_key, file_sha256, file_enc_sha256, file_length, mimetype
		FROM media_refs WHERE conversation_id = ? AND message_id = ?`

	var ref mediaRef
	row := m.db.QueryRowContext(ctx, q, conversationRemoteID, messageRemoteID)
	err := row.Scan(&ref.DirectPath, &ref.MediaKey, &ref.FileSHA256, &ref.FileEncSHA256, &ref.FileLength, &ref.Mimetype)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaRef{}, false, nil
	}
	if err != nil {
		return mediaRef{}, false, fmt.Errorf("whatsapp: load media reference: %w", err)
	}

	return ref, true, nil
}

// close releases the media store's database handle.
func (m *mediaStore) close() error {
	return m.db.Close()
}

// mediaStorePath is where an account's media reference database lives.
func mediaStorePath(dir, accountID string) string {
	return filepath.Join(dir, accountID+"-media.db")
}

// saveMediaRef remembers msg's media reference for later, if it has
// one. Saving is best effort: a failure only means this one message's
// media cannot be downloaded later, and the message's text is reported
// either way.
func saveMediaRef(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, msg *waE2E.Message) {
	ref, ok := downloadRef(msg)
	if !ok {
		return
	}

	_ = media.put(ctx, conversationRemoteID, messageRemoteID, ref)
}
