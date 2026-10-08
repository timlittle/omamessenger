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
// account otherwise. A message that never reached its service - a
// failed send, with nothing on the far side to remove - is deleted
// locally with no connector call at all. A connector that refuses,
// such as a channel that only ever deletes for everyone or a message
// too old to revoke, reports a safe message the UI can show.
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

	messages, err := c.messagesToDelete(ctx, conv.ID, messageIDs)
	if err != nil {
		return err
	}

	if remoteIDs := remoteIDsOf(messages); len(remoteIDs) > 0 {
		if err := c.deleter.DeleteMessages(ctx, conv, remoteIDs, forEveryone); err != nil {
			return deleteError(err)
		}
	}

	return c.removeDeleted(ctx, conv, messages)
}

// messagesToDelete resolves messageIDs to their stored messages,
// rejecting any from another conversation. A message that never
// reached the service - a failed send - needs no remote id to be
// deleted; one that did reach it must have one, or there is nothing to
// tell the service to delete.
func (c *Commands) messagesToDelete(ctx context.Context, conversationID string, messageIDs []string) ([]domain.Message, error) {
	messages := make([]domain.Message, 0, len(messageIDs))
	for _, id := range messageIDs {
		m, err := c.store.Message(ctx, id)
		if err != nil {
			return nil, err
		}
		if m.ConversationID != conversationID {
			return nil, fmt.Errorf("%w: messageIds must all belong to the conversation", ErrInvalidInput)
		}
		if m.RemoteID == "" && !neverReachedService(m) {
			return nil, fmt.Errorf("%w: this message cannot be deleted yet", ErrInvalidInput)
		}

		messages = append(messages, m)
	}

	return messages, nil
}

// neverReachedService reports whether m is an outgoing message the
// service was never told about: a failed send, which has nothing on
// the far side to remove, so it can be deleted locally with no remote
// id at all.
func neverReachedService(m domain.Message) bool {
	return m.Outgoing && m.Status == domain.StatusFailed
}

// remoteIDsOf lists the service ids of messages that have one.
func remoteIDsOf(messages []domain.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.RemoteID != "" {
			ids = append(ids, m.RemoteID)
		}
	}

	return ids
}

// removeDeleted drops the deleted messages from the store, drops each
// one's outgoing attachment copy if it still has one - a failed send
// never retired its copy the way a confirmed delivery does - and
// publishes their removal and the conversations it left changed.
func (c *Commands) removeDeleted(ctx context.Context, conv domain.Conversation, messages []domain.Message) error {
	before := c.events.unreadTotal(ctx)

	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}

	deleted, err := c.store.DeleteMessagesByID(ctx, ids)
	if err != nil {
		return err
	}

	for _, m := range deleted {
		c.events.publish(ctx, EventMessageRemoved, MessageRemoved{ConversationID: m.ConversationID, MessageID: m.ID})
		c.removeOutgoingAttachment(ctx, m)
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
