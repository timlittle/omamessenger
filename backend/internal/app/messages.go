package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// MaxPageSize is the most messages one Messages call returns.
const MaxPageSize = 200

// Messages returns up to limit messages before beforeID, oldest first, and
// whether older ones remain. A zero limit means the default page size.
func (c *Commands) Messages(ctx context.Context, conversationID, beforeID string, limit int) ([]domain.Message, bool, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, false, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}

	if limit < 0 || limit > MaxPageSize {
		return nil, false, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, MaxPageSize)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return nil, false, err
	}

	page, more, err := c.store.Messages(ctx, conversationID, beforeID, limit)
	if err != nil || more || c.history == nil {
		return page, more, err
	}

	return c.olderFromService(ctx, conv, beforeID, limit, page)
}

// olderFromService fetches history the store does not have yet from the
// conversation's service, then pages again. Older history is not new, so
// the conversation keeps the unread count it had. If the service cannot
// be reached, the page already loaded stands: scrolling back is not worth
// an error.
func (c *Commands) olderFromService(ctx context.Context, conv domain.Conversation, beforeID string, limit int, page []domain.Message) ([]domain.Message, bool, error) {
	oldest, err := c.store.OldestRemoteID(ctx, conv.ID)
	if err != nil {
		return page, false, nil
	}

	totalBefore := c.events.unreadTotal(ctx)
	loaded, err := c.history.LoadOlder(ctx, conv, oldest, max(limit, store.DefaultPageSize))
	if err != nil || loaded == 0 {
		return page, false, nil
	}

	if changed, _ := c.store.SetUnread(ctx, conv.ID, conv.Unread); changed { // a failure leaves a count the next sync corrects
		c.events.conversationChanged(ctx, conv.ID, totalBefore)
	}

	page, _, err = c.store.Messages(ctx, conv.ID, beforeID, limit)

	// The service may hold more still; the next page asks it again.
	return page, true, err
}

// Send stores a message as pending, publishes it and hands it to the
// service. If the service refuses it, the message is returned as failed,
// ready to retry; that is not an error.
func (c *Commands) Send(ctx context.Context, conversationID, text string) (domain.Message, error) {
	text, err := domain.NormalizeOutgoingText(text)
	if err != nil {
		return domain.Message{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Message{}, err
	}

	before := c.events.unreadTotal(ctx)
	m, _, err := c.store.AddMessage(ctx, domain.Message{
		ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: time.Now().UnixMilli(),
	})
	if err != nil {
		return domain.Message{}, err
	}

	c.events.publish(ctx, EventMessageAdded, m)
	c.events.conversationChanged(ctx, conv.ID, before)

	return c.dispatch(ctx, conv, m)
}

// Retry sends a failed outgoing message again.
func (c *Commands) Retry(ctx context.Context, messageID string) (domain.Message, error) {
	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return m, err
	}

	if !m.Outgoing || m.Status != domain.StatusFailed {
		return m, fmt.Errorf("%w: only failed outgoing messages can be retried", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return m, err
	}

	m, err = c.setStatus(ctx, m, domain.StatusPending)
	if err != nil {
		return m, err
	}

	return c.dispatch(ctx, conv, m)
}

// dispatch hands a pending message to the service, marking it failed if
// the service refuses it.
func (c *Commands) dispatch(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	if err := c.dispatcher.Send(ctx, conv, m); err != nil {
		return c.setStatus(ctx, m, domain.StatusFailed)
	}

	return m, nil
}

// setStatus records a delivery status and publishes the change.
func (c *Commands) setStatus(ctx context.Context, m domain.Message, status string) (domain.Message, error) {
	updated, changed, err := c.store.UpdateMessageStatus(ctx, m.ID, status)
	if err != nil {
		return m, err
	}

	if changed {
		c.events.publish(ctx, EventMessageUpdated, updated)
	}

	return updated, nil
}
