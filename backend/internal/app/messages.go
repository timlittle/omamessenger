package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type MessagesListParams struct {
	ConversationID string `json:"conversationId"`
	Before         string `json:"before"`
	Limit          int    `json:"limit"`
}

type MessagesListResult struct {
	Messages []domain.Message `json:"messages"`
	HasMore  bool             `json:"hasMore"`
}

func (c *Commands) MessagesList(_ context.Context, params MessagesListParams) (MessagesListResult, error) {
	if strings.TrimSpace(params.ConversationID) == "" {
		return MessagesListResult{}, fmt.Errorf("%w: conversationId is required", ErrBadRequest)
	}
	limit := params.Limit
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maxPageSize {
		return MessagesListResult{}, fmt.Errorf("%w: message limit must be between 1 and %d", ErrBadRequest, maxPageSize)
	}
	if _, err := c.repo.Conversation(params.ConversationID); err != nil {
		return MessagesListResult{}, err
	}
	messages, hasMore, err := c.repo.Messages(params.ConversationID, params.Before, limit)
	return MessagesListResult{Messages: messages, HasMore: hasMore}, err
}

type SendMessageParams struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
}

// SendMessage stores the message as pending, publishes it, and hands it to
// the owning connector. A connector that refuses it marks it failed.
func (c *Commands) SendMessage(ctx context.Context, params SendMessageParams) (domain.Message, error) {
	if strings.TrimSpace(params.ConversationID) == "" {
		return domain.Message{}, fmt.Errorf("%w: conversationId is required", ErrBadRequest)
	}
	text, err := domain.NormalizeOutgoingText(params.Text)
	if err != nil {
		return domain.Message{}, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	conv, err := c.repo.Conversation(params.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	before := c.pub.unreadTotal()
	message, inserted, err := c.repo.AddMessage(domain.Message{
		ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: c.clock.Now().UnixMilli(),
	})
	if err != nil {
		return domain.Message{}, err
	}
	if inserted {
		c.pub.send("message.added", message)
		_, _ = c.pub.conversation(conv.ID)
		c.pub.unreadChanged(before)
	}
	return c.dispatch(ctx, conv, message)
}

type RetryParams struct {
	MessageID string `json:"messageId"`
}

// Retry returns a failed outgoing message to pending and sends it again.
func (c *Commands) Retry(ctx context.Context, params RetryParams) (domain.Message, error) {
	message, err := c.repo.Message(params.MessageID)
	if err != nil {
		return message, err
	}
	if !message.Outgoing || message.Status != domain.StatusFailed {
		return message, fmt.Errorf("%w: only failed outgoing messages can be retried", ErrBadRequest)
	}
	conv, err := c.repo.Conversation(message.ConversationID)
	if err != nil {
		return message, err
	}
	updated, changed, err := c.repo.UpdateMessageStatus(message.ID, domain.StatusPending)
	if err != nil {
		return message, err
	}
	if changed {
		c.pub.send("message.updated", updated)
	}
	return c.dispatch(ctx, conv, updated)
}

// dispatch hands a pending message to the connector, marking it failed when
// there is no dispatcher or the connector refuses it.
func (c *Commands) dispatch(ctx context.Context, conv domain.Conversation, message domain.Message) (domain.Message, error) {
	d := c.currentDispatcher()
	if d == nil {
		return c.failOutgoing(message, errNoDispatcher)
	}
	if err := d.Send(ctx, conv, message); err != nil {
		return c.failOutgoing(message, err)
	}
	return message, nil
}

func (c *Commands) failOutgoing(message domain.Message, cause error) (domain.Message, error) {
	updated, changed, err := c.repo.UpdateMessageStatus(message.ID, domain.StatusFailed)
	if err != nil {
		return message, errors.Join(cause, err)
	}
	if changed {
		c.pub.send("message.updated", updated)
	}
	return updated, cause
}
