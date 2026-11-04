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

// senderRefresh bundles accountID, senderRemoteID, the corrected name
// and whether it is verified - every argument renameSender and
// refreshReplyQuotes both need - so passing ctx and tx alongside them
// never pushes either past this codebase's own argument limit.
type senderRefresh struct {
	accountID, senderRemoteID, name string
	verified                        bool
}

// RefreshSenderName updates every message senderRemoteID sent, within
// account, to name and verified, and brings each conversation whose
// preview this changes back in step. verified is senderRemoteID's own
// domain.Message.SenderVerified: true when name was resolved from a
// saved contact or a verified business name, false for a bare,
// self-chosen one, so a message already stored under the weaker kind
// is corrected the same way its name is (see docs/decisions.md). It
// returns the local id of every conversation whose preview_sender this
// updated, so the caller can report it again. A sender id is only
// unique within one account, so this never touches another account's
// conversations even if their own remote ids happen to collide.
func (s *Store) RefreshSenderName(ctx context.Context, accountID, senderRemoteID, name string, verified bool) ([]string, error) {
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

	refresh := senderRefresh{accountID: accountID, senderRemoteID: senderRemoteID, name: name, verified: verified}
	if err := renameSender(ctx, tx, refresh); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	if err := refreshPreviewSenders(ctx, tx, accountID, senderRemoteID); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	if err := refreshReplyQuotes(ctx, tx, refresh); err != nil {
		return nil, wrap("refresh sender name", err)
	}

	return changed, wrap("refresh sender name", tx.Commit())
}

// renameSender updates r's sender's name and verified flag on every
// message they sent within r's own account.
func renameSender(ctx context.Context, tx *sql.Tx, r senderRefresh) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE messages SET sender_name=?, sender_verified=?
		WHERE sender_id=? AND (sender_name<>? OR sender_verified<>?)
			AND conversation_id IN (SELECT id FROM conversations WHERE account_id=?)`,
		r.name, r.verified, r.senderRemoteID, r.name, r.verified, r.accountID)

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
		UPDATE conversations SET
			preview_sender=COALESCE((
				SELECT sender_name FROM messages WHERE conversation_id=conversations.id ORDER BY created DESC, rowid DESC LIMIT 1
			), ''),
			preview_sender_verified=COALESCE((
				SELECT sender_verified FROM messages WHERE conversation_id=conversations.id ORDER BY created DESC, rowid DESC LIMIT 1
			), 0)
		WHERE account_id=?
			AND id IN (SELECT conversation_id FROM messages WHERE sender_id=?)`,
		accountID, senderRemoteID)

	return err
}

// refreshReplyQuotes brings every reply quote naming senderRemoteID as
// its sender back in step with renameSender, within the conversations
// senderRemoteID has a message in: a reply keeps its own copy of the
// quoted message's sender id, name and verified flag, resolved once
// when it was first stored, within its own conversation (see
// resolveReply), so correcting that sender's own messages is not
// enough on its own - a reply already quoting one of them would
// otherwise keep showing whatever name, and whatever "~" marking, was
// known at the time forever. The query is scoped to senderRemoteID's own
// conversations, and staleQuotes then matches each candidate quote's own
// stored sender id, never its quoted message's remote id: a remote id is
// only unique within one conversation, so matching on that alone let a
// rename reach a quote in a conversation senderRemoteID never sent that
// message in (see docs/decisions.md). A quote with no stored sender id -
// one whose quote could not be resolved when it was first stored - is
// never rewritten.
func refreshReplyQuotes(ctx context.Context, tx *sql.Tx, r senderRefresh) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, reply_to FROM messages
		WHERE reply_to<>'' AND conversation_id IN (
			SELECT m.conversation_id FROM messages m
			JOIN conversations c ON c.id = m.conversation_id
			WHERE m.sender_id=? AND c.account_id=?
		)`, r.senderRemoteID, r.accountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	fixes, err := staleQuotes(rows, r.senderRemoteID, r.name, r.verified)
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
// returns the fix for every one whose own stored SenderID is
// senderRemoteID, under a name or a verified flag other than name and
// verified. A quote with no stored SenderID is never matched.
func staleQuotes(rows *sql.Rows, senderRemoteID, name string, verified bool) ([]quotedMessageFix, error) {
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
		if reply.SenderID == "" || reply.SenderID != senderRemoteID {
			continue
		}
		if reply.SenderName == name && reply.SenderVerified == verified {
			continue
		}

		reply.SenderName, reply.SenderVerified = name, verified
		encoded, err := json.Marshal(reply)
		if err != nil {
			return nil, err
		}
		fixes = append(fixes, quotedMessageFix{id, string(encoded)})
	}

	return fixes, rows.Err()
}
