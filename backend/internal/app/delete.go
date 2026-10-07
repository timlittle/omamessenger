package app

// delete.go implements the UI's request to delete messages: it asks the
// conversation's service first, through the Deleter the helper wired up
// (see connector.Manager.DeleteMessages), then removes them from the
// store and publishes their removal the same way a deletion the service
// reports on its own does (see Ingest.Deleted).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// DeleteMessages deletes messages from a conversation: for everyone,
// through its service, when forEveryone is true, or only for this
// account otherwise. A connector that refuses, such as a channel that
// only ever deletes for everyone or a message too old to revoke,
// reports a safe message the UI can show.
func (c *Commands) DeleteMessages(ctx context.Context, conversationID string, messageIDs []string, forEveryone bool) error {
	if strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}
	if len(messageIDs) == 0 {
		return fmt.Errorf("%w: messageIds is required", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}

	remoteIDs, err := c.remoteIDsToDelete(ctx, conv.ID, messageIDs)
	if err != nil {
		return err
	}

	if err := c.deleter.DeleteMessages(ctx, conv, remoteIDs, forEveryone); err != nil {
		return deleteError(err)
	}

	return c.removeDeleted(ctx, conv, remoteIDs)
}

// remoteIDsToDelete resolves messageIDs to the service ids Deleter
// needs, rejecting any message from another conversation or one the
// service was never told about yet.
func (c *Commands) remoteIDsToDelete(ctx context.Context, conversationID string, messageIDs []string) ([]string, error) {
	ids := make([]string, 0, len(messageIDs))
	for _, id := range messageIDs {
		m, err := c.store.Message(ctx, id)
		if err != nil {
			return nil, err
		}
		if m.ConversationID != conversationID {
			return nil, fmt.Errorf("%w: messageIds must all belong to the conversation", ErrInvalidInput)
		}
		if m.RemoteID == "" {
			return nil, fmt.Errorf("%w: this message cannot be deleted yet", ErrInvalidInput)
		}

		ids = append(ids, m.RemoteID)
	}

	return ids, nil
}

// removeDeleted drops the deleted messages from the store and publishes
// their removal and the conversations it left changed.
func (c *Commands) removeDeleted(ctx context.Context, conv domain.Conversation, remoteIDs []string) error {
	before := c.events.unreadTotal(ctx)

	deleted, err := c.store.DeleteMessages(ctx, conv.AccountID, []string{conv.RemoteID}, remoteIDs)
	if err != nil {
		return err
	}

	for _, m := range deleted {
		c.events.publish(ctx, EventMessageRemoved, MessageRemoved{ConversationID: m.ConversationID, MessageID: m.ID})
	}
	c.events.conversationChanged(ctx, conv.ID, before)

	return nil
}

// deleteError turns a connector's refusal the user can act on into
// ErrInvalidInput with a safe message; anything else, such as a network
// failure, is returned unchanged so it becomes a fixed internal error.
func deleteError(err error) error {
	if errors.Is(err, connector.ErrNoDeleter) || errors.Is(err, connector.ErrDeleteUnsupported) {
		return fmt.Errorf("%w: this message cannot be deleted", ErrInvalidInput)
	}

	return err
}
