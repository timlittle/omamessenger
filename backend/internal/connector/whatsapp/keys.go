package whatsapp

// keys.go persists each message's sender and whether this account sent
// it, in the same per-account database as the media store (see
// storage.go), so React and reply sending can rebuild the key WhatsApp
// needs to act on a message after this connector's own run that first
// saw it has ended: the chat, who sent it, and whether that was this
// account.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
)

// messageKeySchema creates the table ensureMessageKeysTable installs, if
// it does not already exist.
const messageKeySchema = `CREATE TABLE IF NOT EXISTS message_keys (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	sender_id       TEXT NOT NULL,
	from_me         INTEGER NOT NULL,
	PRIMARY KEY (conversation_id, message_id)
)`

// ensureMessageKeysTable creates the message_keys table in db, if it
// does not already exist.
func ensureMessageKeysTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, messageKeySchema); err != nil {
		return fmt.Errorf("whatsapp: prepare message key store: %w", err)
	}

	return nil
}

// messageKey is the sender and from-me flag a message was last seen
// with, enough to rebuild the key WhatsApp needs to react to or quote
// it. senderID is "" for a message this account sent: fromMe alone
// identifies the sender then.
type messageKey struct {
	senderID string
	fromMe   bool
}

// putMessageKey records messageRemoteID's sender and from-me flag,
// replacing whatever was saved for it before.
func (m *mediaStore) putMessageKey(ctx context.Context, conversationRemoteID, messageRemoteID, senderID string, fromMe bool) error {
	const q = `INSERT INTO message_keys (conversation_id, message_id, sender_id, from_me)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET
			sender_id = excluded.sender_id, from_me = excluded.from_me`

	if _, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID, senderID, fromMe); err != nil {
		return fmt.Errorf("whatsapp: save message key: %w", err)
	}

	return nil
}

// messageKeyFor returns the sender and from-me flag saved for a
// message, or false when none was saved for it.
func (m *mediaStore) messageKeyFor(ctx context.Context, conversationRemoteID, messageRemoteID string) (messageKey, bool, error) {
	const q = `SELECT sender_id, from_me FROM message_keys WHERE conversation_id = ? AND message_id = ?`

	var key messageKey
	row := m.db.QueryRowContext(ctx, q, conversationRemoteID, messageRemoteID)
	err := row.Scan(&key.senderID, &key.fromMe)
	if errors.Is(err, sql.ErrNoRows) {
		return messageKey{}, false, nil
	}
	if err != nil {
		return messageKey{}, false, fmt.Errorf("whatsapp: load message key: %w", err)
	}

	return key, true, nil
}

// saveMessageKey remembers a message's sender and from-me flag for
// later, best effort: a failure, or a nil media store such as a test
// connector built without one, only means a later react or reply to
// this one message cannot build more than the plain stanza-id fallback,
// and the message is reported either way.
func saveMessageKey(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, key messageKey) {
	if media == nil {
		return
	}

	_ = media.putMessageKey(ctx, conversationRemoteID, messageRemoteID, key.senderID, key.fromMe)
}

// senderKeyID is the JID message_keys stores for a message not sent by
// this account, needed to react to or quote it correctly in a group. A
// message this account sent needs none: fromMe alone identifies it.
func senderKeyID(info types.MessageInfo) string {
	if info.IsFromMe {
		return ""
	}

	return remoteID(info.Sender)
}

// targetKey builds the key WhatsApp uses to identify a message by its
// chat, id and sender, from whichever of those message_keys last
// recorded for it. participant is set only for a group chat whose
// message was not sent by this account: a direct chat's two parties are
// already identified by fromMe alone, matching how whatsmeow's own
// BuildMessageKey decides when to include it.
func targetKey(chat types.JID, key messageKey, messageRemoteID string) *waCommon.MessageKey {
	built := &waCommon.MessageKey{
		RemoteJID: strp(chat.String()),
		FromMe:    boolp(key.fromMe),
		ID:        strp(messageRemoteID),
	}

	if !key.fromMe && chat.Server == types.GroupServer && key.senderID != "" {
		built.Participant = strp(key.senderID)
	}

	return built
}

// boolp takes the address of a bool, for the generated protobuf structs
// that hold every optional field as a pointer.
func boolp(b bool) *bool { return &b }
