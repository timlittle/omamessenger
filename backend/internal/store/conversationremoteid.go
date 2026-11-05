package store

// This file isolates upgrading a conversation's remote id, split out of
// conversations.go: a connector resolving a better id for a chat it
// already knows under an older one (see UpgradeConversationRemoteID).

import (
	"context"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// UpgradeConversationRemoteID changes a conversation's remote id in place,
// keeping its local id, messages and settings, for a connector that
// resolves a better id for one already stored under an older one. If a
// conversation already exists under newRemoteID, the two are merged
// instead. It reports domain.ErrNotFound when no conversation has
// oldRemoteID.
func (s *Store) UpgradeConversationRemoteID(ctx context.Context, accountID, oldRemoteID, newRemoteID string) (domain.Conversation, error) {
	if oldRemoteID == newRemoteID {
		return s.ConversationByRemote(ctx, accountID, oldRemoteID)
	}

	old, err := s.ConversationByRemote(ctx, accountID, oldRemoteID)
	if err != nil {
		return domain.Conversation{}, err
	}

	target, err := s.ConversationByRemote(ctx, accountID, newRemoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return s.renameConversationRemoteID(ctx, old.ID, newRemoteID)
	}
	if err != nil {
		return domain.Conversation{}, err
	}

	return s.mergeConversations(ctx, old.ID, target.ID)
}

// renameConversationRemoteID points a conversation at a new remote id,
// for UpgradeConversationRemoteID's simple case: nothing is stored
// under the new id yet.
func (s *Store) renameConversationRemoteID(ctx context.Context, id, remoteID string) (domain.Conversation, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE conversations SET remote_id=? WHERE id=?`, remoteID, id); err != nil {
		return domain.Conversation{}, wrap("rename conversation remote id", err)
	}

	return s.Conversation(ctx, id)
}

// mergeConversations moves fromID's messages into intoID and removes fromID.
// A message under a remote id intoID already has is left behind and
// discarded, since moving it would violate the unique index.
func (s *Store) mergeConversations(ctx context.Context, fromID, intoID string) (domain.Conversation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE OR IGNORE messages SET conversation_id=? WHERE conversation_id=?`, intoID, fromID); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id=?`, fromID); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	return s.Conversation(ctx, intoID)
}
