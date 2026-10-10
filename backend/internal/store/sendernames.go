package store

// sendernames.go corrects a sender's name after it was already stored
// on their earlier messages: a connector reports a message with
// whatever name it can resolve at the time (see
// domain.Message.SenderName), and a contact, push or business name
// often resolves only afterwards. Without this, a conversation's
// preview keeps showing that first, often generic, name forever,
// since bumpConversation only ever sets it once, when a message
// arrives.

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// RefreshSenderName updates every message senderRemoteID sent, within
// account, to name, and brings each conversation whose preview this
// changes back in step. It returns the local id of every conversation
// whose preview_sender this updated, so the caller can report it
// again. A sender id is only unique within one account, so this never
// touches another account's conversations even if their own remote
// ids happen to collide.
func (s *Store) RefreshSenderName(ctx context.Context, accountID, senderRemoteID, name string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap("refresh sender name", err)
	}
	// Committed on success below; a rollback after that is a no-op and
	// carries no information worth checking.
	defer func() { _ = tx.Rollback() }()

	changed, err := changedPreviews(ctx, tx, accountID, senderRemoteID, name)
	if err != nil {
		return nil, wrap("refresh sender name", err)
	}

	if err := renameSender(ctx, tx, accountID, senderRemoteID, name); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	if err := refreshPreviewSenders(ctx, tx, accountID, senderRemoteID); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	if err := refreshReplyQuotes(ctx, tx, accountID, senderRemoteID, name); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	return changed, wrap("refresh sender name", tx.Commit())
}

// renameSender updates senderRemoteID's name on every message they
// sent within account.
func renameSender(ctx context.Context, tx *sql.Tx, accountID, senderRemoteID, name string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE messages SET sender_name=?
		WHERE sender_id=? AND sender_name<>?
			AND conversation_id IN (SELECT id FROM conversations WHERE account_id=?)`,
		name, senderRemoteID, name, accountID)

	return err
}

// changedPreviews lists the local id of every one of account's
// conversations senderRemoteID has a message in, whose preview_sender
// does not already match name: read before renameSender and
// refreshPreviewSenders apply it, so the comparison is against the
// name stored before this call.
func changedPreviews(ctx context.Context, tx *sql.Tx, accountID, senderRemoteID, name string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id FROM conversations c
		WHERE c.account_id=? AND c.preview_sender<>?
			AND c.id IN (
				SELECT m.conversation_id FROM messages m
				WHERE m.sender_id=? AND m.rowid=(
					SELECT rowid FROM messages WHERE conversation_id=m.conversation_id ORDER BY created DESC, rowid DESC LIMIT 1
				)
			)`,
		accountID, name, senderRemoteID)
	if err != nil {
		return nil, err
	}

	return scanAll(rows, scanConversationID)
}

// scanConversationID reads a single conversation id column, for
// changedPreviews.
func scanConversationID(row scanner) (string, error) {
	var id string
	err := row.Scan(&id)

	return id, err
}

// refreshPreviewSenders recomputes preview_sender, from each
// conversation's own newest message, for every one of account's
// conversations senderRemoteID has a message in. It is called after
// renameSender, so a conversation whose newest message was theirs
// picks up the refreshed name.
func refreshPreviewSenders(ctx context.Context, tx *sql.Tx, accountID, senderRemoteID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE conversations SET preview_sender=COALESCE((
			SELECT sender_name FROM messages WHERE conversation_id=conversations.id ORDER BY created DESC, rowid DESC LIMIT 1
		), '')
		WHERE account_id=?
			AND id IN (SELECT conversation_id FROM messages WHERE sender_id=?)`,
		accountID, senderRemoteID)

	return err
}

// refreshReplyQuotes brings every reply quote naming senderRemoteID as
// its sender back in step with renameSender, within account: a reply
// keeps its own copy of the quoted message's sender name, resolved once
// when it was first stored (see resolveReply), so correcting that
// sender's own messages is not enough on its own - a reply already
// quoting one of them would otherwise keep showing whatever name was
// known at the time forever.
func refreshReplyQuotes(ctx context.Context, tx *sql.Tx, accountID, senderRemoteID, name string) error {
	remotes, err := sentRemoteIDs(ctx, tx, accountID, senderRemoteID)
	if err != nil || len(remotes) == 0 {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, reply_to FROM messages
		WHERE reply_to<>'' AND conversation_id IN (SELECT id FROM conversations WHERE account_id=?)`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	fixes, err := staleQuotes(rows, remotes, name)
	if err != nil {
		return err
	}

	for _, f := range fixes {
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET reply_to=? WHERE id=?`, f.replyTo, f.id); err != nil {
			return err
		}
	}

	return nil
}

// quotedMessageFix is one reply quote refreshReplyQuotes found stale,
// with its corrected JSON ready to write back.
type quotedMessageFix struct {
	id, replyTo string
}

// staleQuotes scans rows of a message's id and its reply_to JSON, and
// returns the fix for every one that quotes a message in remotes under
// a name other than name.
func staleQuotes(rows *sql.Rows, remotes map[string]bool, name string) ([]quotedMessageFix, error) {
	var fixes []quotedMessageFix
	for rows.Next() {
		var id, replyTo string
		if err := rows.Scan(&id, &replyTo); err != nil {
			return nil, err
		}

		var reply domain.Reply
		if err := json.Unmarshal([]byte(replyTo), &reply); err != nil {
			continue
		}
		if !remotes[reply.RemoteID] || reply.SenderName == name {
			continue
		}

		reply.SenderName = name
		encoded, err := json.Marshal(reply)
		if err != nil {
			return nil, err
		}
		fixes = append(fixes, quotedMessageFix{id, string(encoded)})
	}

	return fixes, rows.Err()
}

// sentRemoteIDs lists the remote ids, within account, that senderRemoteID
// has a message stored under, so refreshReplyQuotes can tell whether a
// stored reply quotes one of them.
func sentRemoteIDs(ctx context.Context, tx *sql.Tx, accountID, senderRemoteID string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT remote_id FROM messages
		WHERE sender_id=? AND remote_id<>'' AND conversation_id IN (SELECT id FROM conversations WHERE account_id=?)`,
		senderRemoteID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}

	return out, rows.Err()
}
