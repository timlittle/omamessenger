package store

// messages_delete.go removes messages a service reports deleted,
// keeping each touched conversation's preview, activity and unread
// count in step with what remains.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// DeleteMessages removes messages a service reports deleted, found by the
// ids it gave them, within conversationRemoteIDs: the conversations of
// the account the ids might belong to. The caller names them explicitly
// because the store has no notion of a service's own rules for when an id
// is unique to one conversation or shared account-wide; a remote id that
// is not stored in one of them is ignored. Each conversation a removal
// touches has its preview and activity fall back to its newest remaining
// message, and its unread count drops by one for every removed message
// that was unread. deleted lists the removed messages, each still
// carrying its conversation id.
func (s *Store) DeleteMessages(ctx context.Context, accountID string, conversationRemoteIDs, remoteIDs []string) ([]domain.Message, error) {
	if len(remoteIDs) == 0 {
		return nil, nil
	}

	convIDs, err := s.deletionScope(ctx, accountID, conversationRemoteIDs)
	if err != nil || len(convIDs) == 0 {
		return nil, err
	}

	deleted, err := s.findDeletable(ctx, convIDs, remoteIDs)
	if err != nil || len(deleted) == 0 {
		return nil, err
	}

	if err := s.removeMessages(ctx, deleted); err != nil {
		return nil, wrap("delete messages", err)
	}

	return deleted, nil
}

// deletionScope resolves each of conversationRemoteIDs to its local
// conversation id, skipping any that is not a conversation of the
// account.
func (s *Store) deletionScope(ctx context.Context, accountID string, conversationRemoteIDs []string) ([]string, error) {
	ids := make([]string, 0, len(conversationRemoteIDs))
	for _, remote := range conversationRemoteIDs {
		conv, err := s.ConversationByRemote(ctx, accountID, remote)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}

		ids = append(ids, conv.ID)
	}

	return ids, nil
}

// findDeletable returns the stored messages among remoteIDs, in any of
// convIDs.
func (s *Store) findDeletable(ctx context.Context, convIDs, remoteIDs []string) ([]domain.Message, error) {
	//nolint:gosec // deliberate: messageColumns and placeholders() are fixed strings this package builds; every actual value is a bound ? argument
	query := `SELECT ` + messageColumns + ` FROM messages
		WHERE remote_id<>'' AND conversation_id IN (` + placeholders(len(convIDs)) + `)
		AND remote_id IN (` + placeholders(len(remoteIDs)) + `)`

	args := make([]any, 0, len(convIDs)+len(remoteIDs))
	for _, id := range convIDs {
		args = append(args, id)
	}
	for _, id := range remoteIDs {
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap("find deletable", err)
	}

	messages, err := scanAll(rows, scanMessage)

	return messages, wrap("find deletable", err)
}

// removeMessages deletes messages and brings each conversation they left
// up to date: preview, activity and unread count.
func (s *Store) removeMessages(ctx context.Context, messages []domain.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	touched := map[string]bool{}
	for _, m := range messages {
		if err := deleteOneMessage(ctx, tx, m); err != nil {
			return err
		}
		touched[m.ConversationID] = true
	}

	for conv := range touched {
		if err := refreshAfterDeletion(ctx, tx, conv); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// deleteOneMessage removes a message and lowers its conversation's unread
// count when the message was counted in it.
func deleteOneMessage(ctx context.Context, tx *sql.Tx, m domain.Message) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id=?`, m.ID); err != nil {
		return err
	}

	if m.Outgoing || m.Status == domain.StatusRead {
		return nil
	}

	_, err := tx.ExecContext(ctx, `UPDATE conversations SET unread=MAX(unread-1,0) WHERE id=?`, m.ConversationID)

	return err
}

// refreshAfterDeletion recomputes a conversation's preview and activity
// from whatever messages it has left, after one or more were removed.
func refreshAfterDeletion(ctx context.Context, tx *sql.Tx, conversationID string) error {
	const newest = `SELECT %s FROM messages WHERE conversation_id=? ORDER BY created DESC, rowid DESC LIMIT 1`
	//nolint:gosec // deliberate: newest's %s is always one of this function's own fixed column-name literals below, never untrusted input
	_, err := tx.ExecContext(ctx, `UPDATE conversations SET
			preview=COALESCE((`+fmt.Sprintf(newest, "text")+`),''),
			preview_sender=COALESCE((`+fmt.Sprintf(newest, "sender_name")+`),''),
			preview_out=COALESCE((`+fmt.Sprintf(newest, "outgoing")+`),0),
			last_activity=COALESCE((`+fmt.Sprintf(newest, "created")+`),0)
		WHERE id=?`,
		conversationID, conversationID, conversationID, conversationID, conversationID)

	return err
}
