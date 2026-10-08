package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Conversations lists conversations, newest first, optionally filtered by a
// search query over titles and messages.
func (c *Commands) Conversations(ctx context.Context, query string) ([]domain.Conversation, error) {
	return c.store.Conversations(ctx, query)
}

// OpenConversation returns the direct conversation with a contact, creating
// it when there is none yet.
func (c *Commands) OpenConversation(ctx context.Context, accountID, contactID string) (domain.Conversation, error) {
	if accountID == "" || contactID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: accountId and contactId are required", ErrInvalidInput)
	}

	contact, err := c.store.Contact(ctx, accountID, contactID)
	if err != nil {
		return domain.Conversation{}, err
	}

	conv, created, err := c.store.EnsureConversation(ctx, domain.Conversation{
		AccountID: accountID, RemoteID: contact.RemoteID, Kind: domain.KindDirect, Title: contact.Name,
	})
	if err != nil {
		return conv, err
	}

	if created {
		c.events.publish(ctx, EventConversationUpdated, conv)
	}

	return conv, nil
}

// MarkRead clears a conversation's unread count, then tells the
// service. The service hears the count this conversation actually had
// before clearing it: a connector such as WhatsApp's needs that number
// to know how many of a conversation's newest incoming messages to send
// a read receipt for, and asking the store only after the local clear
// would always report zero, turning every such call into a silent
// no-op for that service.
func (c *Commands) MarkRead(ctx context.Context, conversationID string) error {
	before := c.events.unreadTotal(ctx)

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}

	changed, err := c.store.MarkRead(ctx, conversationID)
	if err != nil {
		return err
	}

	if changed {
		c.events.conversationChanged(ctx, conversationID, before)
	}

	return c.dispatcher.MarkRead(ctx, conv)
}

// SetMuted mutes or unmutes a conversation. Muted conversations do not
// notify or count towards the unread total.
func (c *Commands) SetMuted(ctx context.Context, conversationID string, muted bool) (domain.Conversation, error) {
	before := c.events.unreadTotal(ctx)

	if err := c.store.SetMuted(ctx, conversationID, muted); err != nil {
		return domain.Conversation{}, err
	}

	c.events.conversationChanged(ctx, conversationID, before)

	return c.store.Conversation(ctx, conversationID)
}

// SetPinned pins or unpins a conversation, then tells its service so a
// pin made here, or synced back from another device, agrees everywhere.
// The local change stands even if the service call fails; that failure is
// returned so the caller can tell the user.
func (c *Commands) SetPinned(ctx context.Context, conversationID string, pinned bool) (domain.Conversation, error) {
	if err := c.store.SetPinned(ctx, conversationID, pinned); err != nil {
		return domain.Conversation{}, err
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	c.events.publish(ctx, EventConversationUpdated, conv)

	return conv, pinError(c.organizer.SetPinned(ctx, conv, pinned))
}

// pinError turns a connector's pin limit refusal into ErrInvalidInput
// with a safe message the UI can show; anything else, such as a network
// failure, is returned unchanged so it becomes a fixed internal error.
func pinError(err error) error {
	if errors.Is(err, connector.ErrPinLimit) {
		return fmt.Errorf("%w: this service limits how many conversations can stay pinned", ErrInvalidInput)
	}

	return err
}

// SetArchived files a conversation away, or brings it back, then tells its
// service. The local change stands even if the service call fails; that
// failure is returned so the caller can tell the user.
func (c *Commands) SetArchived(ctx context.Context, conversationID string, archived bool) (domain.Conversation, error) {
	if err := c.store.SetArchived(ctx, conversationID, archived); err != nil {
		return domain.Conversation{}, err
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	c.events.publish(ctx, EventConversationUpdated, conv)

	return conv, c.organizer.SetArchived(ctx, conv, archived)
}

// SetHidden hides or unhides a conversation from the standard list. Unlike
// mute, pin and archive, this is local to OmaMessenger only: it is never
// reported to the service, so no connector call follows.
func (c *Commands) SetHidden(ctx context.Context, conversationID string, hidden bool) (domain.Conversation, error) {
	if err := c.store.SetHidden(ctx, conversationID, hidden); err != nil {
		return domain.Conversation{}, err
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	c.events.publish(ctx, EventConversationUpdated, conv)

	return conv, nil
}
