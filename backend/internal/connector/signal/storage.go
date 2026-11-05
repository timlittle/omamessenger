package signal

// storage.go persists each message's attachment pointer, server GUID,
// report-spam token and poll state, in its own database beside the
// signalmeow session, the way WhatsApp's media and poll stores do.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mediaStore is an account's attachment, message metadata and poll
// database.
type mediaStore struct {
	db *sql.DB
}

// mediaStoreSchema creates every table mediaStore reads and writes, if
// they do not already exist.
const mediaStoreSchema = `
CREATE TABLE IF NOT EXISTS attachment_refs (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	cdn_id          INTEGER NOT NULL,
	cdn_key         TEXT NOT NULL,
	cdn_number      INTEGER NOT NULL,
	enc_key         BLOB NOT NULL,
	digest          BLOB NOT NULL,
	size            INTEGER NOT NULL,
	content_type    TEXT NOT NULL DEFAULT '',
	file_name       TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (conversation_id, message_id)
);
CREATE TABLE IF NOT EXISTS message_meta (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	server_guid     TEXT NOT NULL,
	report_token    BLOB NOT NULL,
	PRIMARY KEY (conversation_id, message_id)
);
CREATE TABLE IF NOT EXISTS poll_meta (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	question        TEXT NOT NULL,
	multiple_choice INTEGER NOT NULL,
	closed          INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (conversation_id, message_id)
);
CREATE TABLE IF NOT EXISTS poll_options (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	option_id       TEXT NOT NULL,
	text            TEXT NOT NULL,
	position        INTEGER NOT NULL,
	PRIMARY KEY (conversation_id, message_id, option_id)
);
CREATE TABLE IF NOT EXISTS poll_votes (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	voter_id        TEXT NOT NULL,
	option_id       TEXT NOT NULL,
	PRIMARY KEY (conversation_id, message_id, voter_id, option_id)
);`

// openMediaStore opens, creating if needed, the database that holds
// account's attachments, message metadata and polls in dir.
func openMediaStore(ctx context.Context, dir, accountID string) (*mediaStore, error) {
	path := mediaStorePath(dir, accountID)
	if err := createPrivateFile(path); err != nil {
		return nil, fmt.Errorf("signal: open media store: %w", err)
	}

	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?_pragma=busy_timeout(5000)"

	return openMediaStoreDSN(ctx, dsn)
}

// openMediaStoreDSN opens a media store from a database/sql data source
// name, shared by openMediaStore's real file and the in-memory one
// tests default to.
func openMediaStoreDSN(ctx context.Context, dsn string) (*mediaStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("signal: open media store: %w", err)
	}

	if _, err := db.ExecContext(ctx, mediaStoreSchema); err != nil {
		_ = db.Close() // the database could not be prepared; nothing to flush
		return nil, fmt.Errorf("signal: prepare media store: %w", err)
	}

	return &mediaStore{db: db}, nil
}

// mediaStorePath is where an account's media store lives.
func mediaStorePath(dir, accountID string) string {
	return filepath.Join(dir, accountID+"-media.db")
}

// close releases the media store's database handle.
func (m *mediaStore) close() error {
	return m.db.Close()
}

// putAttachment saves ptr as the attachment behind messageRemoteID in
// conversationRemoteID, replacing whatever was saved before.
func (m *mediaStore) putAttachment(ctx context.Context, conversationRemoteID, messageRemoteID string, ptr *signalpb.AttachmentPointer) error {
	const q = `INSERT INTO attachment_refs
		(conversation_id, message_id, cdn_id, cdn_key, cdn_number, enc_key, digest, size, content_type, file_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET
			cdn_id = excluded.cdn_id, cdn_key = excluded.cdn_key, cdn_number = excluded.cdn_number,
			enc_key = excluded.enc_key, digest = excluded.digest, size = excluded.size,
			content_type = excluded.content_type, file_name = excluded.file_name`

	// A nil key or digest would insert SQL NULL into a NOT NULL blob
	// column; emptyBytes keeps a pointer missing one, such as a test
	// fixture, storable without weakening the real column constraint.
	_, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID,
		ptr.GetCdnId(), ptr.GetCdnKey(), ptr.GetCdnNumber(), emptyBytes(ptr.GetKey()), emptyBytes(ptr.GetDigest()), ptr.GetSize(),
		ptr.GetContentType(), ptr.GetFileName())
	if err != nil {
		return fmt.Errorf("signal: save attachment: %w", err)
	}

	return nil
}

// emptyBytes turns a nil slice into an empty, non-nil one, so it
// inserts as a blob rather than SQL NULL.
func emptyBytes(b []byte) []byte {
	if b == nil {
		return []byte{}
	}

	return b
}

// attachment returns the attachment pointer saved for a message, or
// found false when none was saved for it.
func (m *mediaStore) attachment(ctx context.Context, conversationRemoteID, messageRemoteID string) (*signalpb.AttachmentPointer, bool, error) {
	const q = `SELECT cdn_id, cdn_key, cdn_number, enc_key, digest, size, content_type, file_name
		FROM attachment_refs WHERE conversation_id = ? AND message_id = ?`

	var ptr signalpb.AttachmentPointer
	var cdnID, cdnNumber, size uint32
	var cdnKey, contentType, fileName string
	var key, digest []byte

	row := m.db.QueryRowContext(ctx, q, conversationRemoteID, messageRemoteID)
	err := row.Scan(&cdnID, &cdnKey, &cdnNumber, &key, &digest, &size, &contentType, &fileName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("signal: load attachment: %w", err)
	}

	ptr.AttachmentIdentifier = &signalpb.AttachmentPointer_CdnKey{CdnKey: cdnKey}
	if cdnID != 0 {
		ptr.AttachmentIdentifier = &signalpb.AttachmentPointer_CdnId{CdnId: uint64(cdnID)}
	}
	ptr.Key, ptr.Digest, ptr.Size = key, digest, &size
	ptr.CdnNumber, ptr.ContentType, ptr.FileName = &cdnNumber, &contentType, &fileName

	return &ptr, true, nil
}

// putMessageMeta saves a message's server GUID and report-spam token,
// replacing whatever was saved before.
func (m *mediaStore) putMessageMeta(ctx context.Context, conversationRemoteID, messageRemoteID, serverGUID string, reportToken []byte) error {
	const q = `INSERT INTO message_meta (conversation_id, message_id, server_guid, report_token) VALUES (?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET server_guid = excluded.server_guid, report_token = excluded.report_token`

	if _, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID, serverGUID, emptyBytes(reportToken)); err != nil {
		return fmt.Errorf("signal: save message metadata: %w", err)
	}

	return nil
}

// messageMeta returns a message's saved server GUID and report-spam
// token, or found false when none was saved for it.
func (m *mediaStore) messageMeta(ctx context.Context, conversationRemoteID, messageRemoteID string) (serverGUID string, reportToken []byte, found bool, err error) {
	const q = `SELECT server_guid, report_token FROM message_meta WHERE conversation_id = ? AND message_id = ?`

	row := m.db.QueryRowContext(ctx, q, conversationRemoteID, messageRemoteID)
	if err := row.Scan(&serverGUID, &reportToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, false, nil
		}

		return "", nil, false, fmt.Errorf("signal: load message metadata: %w", err)
	}

	return serverGUID, reportToken, true, nil
}

// putPoll saves a newly created poll's question, multiple-choice flag
// and option text, replacing whatever was saved before.
func (m *mediaStore) putPoll(ctx context.Context, conversationRemoteID, messageRemoteID, question string, options []string, multipleChoice bool) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("signal: save poll: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // only reached after Commit fails or on an early return, nothing left to flush

	const meta = `INSERT INTO poll_meta (conversation_id, message_id, question, multiple_choice) VALUES (?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET question = excluded.question, multiple_choice = excluded.multiple_choice`
	if _, err := tx.ExecContext(ctx, meta, conversationRemoteID, messageRemoteID, question, multipleChoice); err != nil {
		return fmt.Errorf("signal: save poll: %w", err)
	}

	const option = `INSERT INTO poll_options (conversation_id, message_id, option_id, text, position) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id, option_id) DO UPDATE SET text = excluded.text, position = excluded.position`
	for i, text := range options {
		if _, err := tx.ExecContext(ctx, option, conversationRemoteID, messageRemoteID, formatOptionID(i), text, i); err != nil {
			return fmt.Errorf("signal: save poll: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("signal: save poll: %w", err)
	}

	return nil
}

// setVotes replaces voterID's selected options: Signal always sends
// a voter's complete current choice rather than a delta.
func (m *mediaStore) setVotes(ctx context.Context, conversationRemoteID, messageRemoteID, voterID string, optionIndexes []int) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("signal: save vote: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // only reached after Commit fails or on an early return, nothing left to flush

	const clear = `DELETE FROM poll_votes WHERE conversation_id = ? AND message_id = ? AND voter_id = ?`
	if _, err := tx.ExecContext(ctx, clear, conversationRemoteID, messageRemoteID, voterID); err != nil {
		return fmt.Errorf("signal: save vote: %w", err)
	}

	const insert = `INSERT INTO poll_votes (conversation_id, message_id, voter_id, option_id) VALUES (?, ?, ?, ?)`
	for _, idx := range optionIndexes {
		if idx < 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, insert, conversationRemoteID, messageRemoteID, voterID, formatOptionID(idx)); err != nil {
			return fmt.Errorf("signal: save vote: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("signal: save vote: %w", err)
	}

	return nil
}

// closePoll marks a poll closed, so a later tally reports it so.
func (m *mediaStore) closePoll(ctx context.Context, conversationRemoteID, messageRemoteID string) error {
	const q = `UPDATE poll_meta SET closed = 1 WHERE conversation_id = ? AND message_id = ?`

	if _, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID); err != nil {
		return fmt.Errorf("signal: close poll: %w", err)
	}

	return nil
}

// pollTally rebuilds a poll's question, options and vote counts, or
// found false when no poll was saved for this message.
func (m *mediaStore) pollTally(ctx context.Context, conversationRemoteID, messageRemoteID, selfVoterID string) (domain.Poll, bool, error) {
	const metaQuery = `SELECT question, multiple_choice, closed FROM poll_meta WHERE conversation_id = ? AND message_id = ?`

	var poll domain.Poll
	row := m.db.QueryRowContext(ctx, metaQuery, conversationRemoteID, messageRemoteID)
	if err := row.Scan(&poll.Question, &poll.MultipleChoice, &poll.Closed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Poll{}, false, nil
		}

		return domain.Poll{}, false, fmt.Errorf("signal: load poll: %w", err)
	}

	options, err := m.pollOptions(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return domain.Poll{}, false, err
	}

	votersByOption, totalVoters, err := m.pollVoters(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return domain.Poll{}, false, err
	}

	for i, o := range options {
		voters := votersByOption[o.ID]
		options[i].Votes = len(voters)
		options[i].Chosen = voters[selfVoterID]
	}
	poll.Options = options
	poll.TotalVoters = totalVoters

	return poll, true, nil
}

// pollOptions returns a poll's options, in the order they were saved
// with, each with no tally yet.
func (m *mediaStore) pollOptions(ctx context.Context, conversationRemoteID, messageRemoteID string) ([]domain.PollOption, error) {
	const q = `SELECT option_id, text FROM poll_options WHERE conversation_id = ? AND message_id = ? ORDER BY position`

	rows, err := m.db.QueryContext(ctx, q, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, fmt.Errorf("signal: load poll options: %w", err)
	}
	defer rows.Close()

	var options []domain.PollOption
	for rows.Next() {
		var o domain.PollOption
		if err := rows.Scan(&o.ID, &o.Text); err != nil {
			return nil, fmt.Errorf("signal: load poll options: %w", err)
		}
		options = append(options, o)
	}

	return options, rows.Err()
}

// pollVoters lists, for each option id, the set of voters who chose
// it, and how many distinct voters voted at all.
func (m *mediaStore) pollVoters(ctx context.Context, conversationRemoteID, messageRemoteID string) (map[string]map[string]bool, int, error) {
	const q = `SELECT option_id, voter_id FROM poll_votes WHERE conversation_id = ? AND message_id = ?`

	rows, err := m.db.QueryContext(ctx, q, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, 0, fmt.Errorf("signal: load votes: %w", err)
	}
	defer rows.Close()

	votersByOption := map[string]map[string]bool{}
	everyVoter := map[string]bool{}
	for rows.Next() {
		var optionID, voterID string
		if err := rows.Scan(&optionID, &voterID); err != nil {
			return nil, 0, fmt.Errorf("signal: load votes: %w", err)
		}

		if votersByOption[optionID] == nil {
			votersByOption[optionID] = map[string]bool{}
		}
		votersByOption[optionID][voterID] = true
		everyVoter[voterID] = true
	}

	return votersByOption, len(everyVoter), rows.Err()
}
