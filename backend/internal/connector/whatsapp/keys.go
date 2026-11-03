package whatsapp

// keys.go persists each message's sender, whether this account sent it,
// and when it arrived, in the same per-account database as the media
// store (see storage.go), so React and reply sending can rebuild the key
// WhatsApp needs to act on a message after this connector's own run
// that first saw it has ended: the chat, who sent it, and whether that
// was this account. The timestamp exists for MarkRead (see markread.go):
// it is what lets a restarted process, with nothing of its own left in
// memory, still work out which of a conversation's saved messages are
// its newest incoming ones.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// messageKeySchema creates the table ensureMessageKeysTable installs, if
// it does not already exist.
const messageKeySchema = `CREATE TABLE IF NOT EXISTS message_keys (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	sender_id       TEXT NOT NULL,
	from_me         INTEGER NOT NULL,
	timestamp       INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (conversation_id, message_id)
)`

// ensureMessageKeysTable creates the message_keys table in db, if it
// does not already exist, and adds its timestamp column to one created
// before MarkRead needed it: CREATE TABLE IF NOT EXISTS above never
// adds a column to a table that already exists under the older schema.
func ensureMessageKeysTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, messageKeySchema); err != nil {
		return fmt.Errorf("whatsapp: prepare message key store: %w", err)
	}

	return addMessageKeyTimestampColumn(ctx, db)
}

// addMessageKeyTimestampColumn adds message_keys' timestamp column when
// an account's existing database still lacks it.
func addMessageKeyTimestampColumn(ctx context.Context, db *sql.DB) error {
	has, err := hasMessageKeyTimestampColumn(ctx, db)
	if err != nil || has {
		return err
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE message_keys ADD COLUMN timestamp INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("whatsapp: add message key timestamp: %w", err)
	}

	return nil
}

// hasMessageKeyTimestampColumn reports whether message_keys already has
// its timestamp column.
func hasMessageKeyTimestampColumn(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(message_keys)`)
	if err != nil {
		return false, fmt.Errorf("whatsapp: inspect message key store: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("whatsapp: inspect message key store: %w", err)
		}
		if name == "timestamp" {
			return true, nil
		}
	}

	return false, rows.Err()
}

// messageKey is the sender, from-me flag and timestamp (Unix
// milliseconds, matching domain.Message.Created) a message was last
// seen with. The sender and from-me flag are enough to rebuild the key
// WhatsApp needs to react to or quote it; senderID is "" for a message
// this account sent, since fromMe alone identifies the sender then.
// timestamp exists for MarkRead (see markread.go): it is what lets a
// restarted process, with nothing of its own left in memory, still
// work out which of a conversation's saved messages are its newest
// incoming ones. targetKey and senderKeyID, which only ever build or
// read the first two fields, ignore it.
type messageKey struct {
	senderID  string
	fromMe    bool
	timestamp int64
}

// putMessageKey records messageRemoteID's key: whichever sender and
// from-me flag were saved first for a (conversation, message) id win
// and are never overwritten by a different identity later saved under
// the same id, only ever updated (for instance to refresh the
// timestamp) when the identity being saved already matches what is on
// record. WhatsApp lets whoever sends a stanza choose its own id, so
// without this, a second sender naming someone else's id - or this
// account's own - could have message_keys remember them as that
// message's sender instead, letting them pass authorizedActor's revoke
// or edit check for a message they never sent (see docs/decisions.md).
// A rejected save is not reported as an error: the message itself is
// still processed normally, just never allowed to borrow another
// message's identity.
func (m *mediaStore) putMessageKey(ctx context.Context, conversationRemoteID, messageRemoteID string, key messageKey) error {
	const q = `INSERT INTO message_keys (conversation_id, message_id, sender_id, from_me, timestamp)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET
			sender_id = excluded.sender_id, from_me = excluded.from_me, timestamp = excluded.timestamp
		WHERE message_keys.sender_id = excluded.sender_id AND message_keys.from_me = excluded.from_me`

	// senderKeyID reads a JID straight off the wire, with no length
	// limit of its own; dropping an oversized one to "" rather than
	// truncating it is the same treatment domain.CapRemoteID gives a
	// reply quote's own RemoteID elsewhere, for the same reason: a cut
	// id could read as a different, real, shorter sender it was never
	// actually sent by.
	key.senderID = domain.CapRemoteID(key.senderID)

	res, err := m.db.ExecContext(ctx, q, conversationRemoteID, messageRemoteID, key.senderID, key.fromMe, key.timestamp)
	if err != nil {
		return fmt.Errorf("whatsapp: save message key: %w", err)
	}

	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		logDropped(reasonReusedMessageID)
	}

	return nil
}

// messageKeyFor returns the key saved for a message, or false when none
// was saved for it.
func (m *mediaStore) messageKeyFor(ctx context.Context, conversationRemoteID, messageRemoteID string) (messageKey, bool, error) {
	const q = `SELECT sender_id, from_me, timestamp FROM message_keys WHERE conversation_id = ? AND message_id = ?`

	var key messageKey
	row := m.db.QueryRowContext(ctx, q, conversationRemoteID, messageRemoteID)
	err := row.Scan(&key.senderID, &key.fromMe, &key.timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return messageKey{}, false, nil
	}
	if err != nil {
		return messageKey{}, false, fmt.Errorf("whatsapp: load message key: %w", err)
	}

	return key, true, nil
}

// saveMessageKey remembers a message's key for later, best effort: a
// failure, or a nil media store such as a test connector built without
// one, only means a later react or reply to this one message cannot
// build more than the plain stanza-id fallback, and that MarkRead
// cannot pick it out of the conversation's newest messages after a
// restart; the message is reported either way.
func saveMessageKey(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, key messageKey) {
	if media == nil {
		return
	}

	_ = media.putMessageKey(ctx, conversationRemoteID, messageRemoteID, key)
}

// reusesAnotherSendersID reports whether messageRemoteID already names
// a message_keys row for a different identity (sender and from-me)
// than key: a nil media store, or an id never saved before, is never a
// reuse, since putMessageKey's own first-writer-wins rule only applies
// once something is on record to reuse. A lookup failure is reported
// as a reuse too, the safer answer when there is nothing to check
// against with confidence.
func reusesAnotherSendersID(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, key messageKey) bool {
	if media == nil {
		return false
	}

	existing, found, err := media.messageKeyFor(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return true
	}

	return found && (existing.senderID != key.senderID || existing.fromMe != key.fromMe)
}

// incomingKeyedMessage bundles what saveIncomingKeyedData needs to
// save one message's media reference, key and poll, keeping its own
// argument count within this codebase's limit.
type incomingKeyedMessage struct {
	conversationRemoteID string
	messageRemoteID      string
	key                  messageKey
	msg                  *waE2E.Message
}

// saveIncomingKeyedData saves m's media reference, message key and
// poll, unless m's own id already names a different sender's message
// (see reusesAnotherSendersID): without this check, handleContent's
// and syncMessage's own saves would let a sender who names someone
// else's id - or this account's own - swap the file fetched for it, or
// a poll's question and options, in addition to the revoke and edit
// forgery putMessageKey alone already closes. It reports whether the
// save went ahead, so a caller that would otherwise treat m's id as
// replacing an earlier placeholder (see live.go's resolveUndecryptable)
// can skip that too for an id reused this way.
func saveIncomingKeyedData(ctx context.Context, media *mediaStore, m incomingKeyedMessage) bool {
	if reusesAnotherSendersID(ctx, media, m.conversationRemoteID, m.messageRemoteID, m.key) {
		logDropped(reasonReusedMessageID)
		return false
	}

	saveMediaRef(ctx, media, m.conversationRemoteID, m.messageRemoteID, m.msg)
	saveMessageKey(ctx, media, m.conversationRemoteID, m.messageRemoteID, m.key)
	savePoll(ctx, media, m.conversationRemoteID, m.messageRemoteID, m.msg)

	return true
}

// unreadMessageKeys returns the ids of the newest limit incoming
// (sender, not this account) messages saved for conversationRemoteID,
// by sender, so MarkRead can send a read receipt for each one from
// durable storage alone: WhatsApp's own unread count only ever says how
// many of a conversation's newest messages are unread, never which
// ones by id, the same assumption history sync's own tail used to make
// before this (see docs/decisions.md). limit <= 0 reports nothing
// pending without even querying, since a conversation with nothing
// unread has nothing to pick.
func (m *mediaStore) unreadMessageKeys(ctx context.Context, conversationRemoteID string, limit int) (map[string][]string, error) {
	if limit <= 0 {
		return nil, nil
	}

	const q = `SELECT message_id, sender_id FROM message_keys
		WHERE conversation_id = ? AND from_me = 0
		ORDER BY timestamp DESC, rowid DESC LIMIT ?`

	rows, err := m.db.QueryContext(ctx, q, conversationRemoteID, limit)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: load unread message keys: %w", err)
	}
	defer rows.Close()

	bySender := map[string][]string{}
	for rows.Next() {
		var id, sender string
		if err := rows.Scan(&id, &sender); err != nil {
			return nil, fmt.Errorf("whatsapp: load unread message keys: %w", err)
		}
		bySender[sender] = append(bySender[sender], id)
	}

	return bySender, rows.Err()
}

// latestMessageKey returns the newest message saved for
// conversationRemoteID, from any sender and whether this account sent
// it, so MarkRead can tell WhatsApp's app state which message its "chat
// read up to" mutation covers. found is false for a conversation with
// no saved key yet.
func (m *mediaStore) latestMessageKey(ctx context.Context, conversationRemoteID string) (messageRemoteID string, key messageKey, found bool, err error) {
	const q = `SELECT message_id, sender_id, from_me, timestamp FROM message_keys
		WHERE conversation_id = ? ORDER BY timestamp DESC, rowid DESC LIMIT 1`

	row := m.db.QueryRowContext(ctx, q, conversationRemoteID)
	err = row.Scan(&messageRemoteID, &key.senderID, &key.fromMe, &key.timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return "", messageKey{}, false, nil
	}
	if err != nil {
		return "", messageKey{}, false, fmt.Errorf("whatsapp: load latest message key: %w", err)
	}

	return messageRemoteID, key, true, nil
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
