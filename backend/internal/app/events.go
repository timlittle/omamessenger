package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/store"
)

// Event names sent to the UI.
const (
	EventAccountUpdated      = "account.updated"
	EventConversationUpdated = "conversation.updated"
	EventMessageAdded        = "message.added"
	EventMessageUpdated      = "message.updated"
	EventUnreadChanged       = "unread.changed"
	EventTyping              = "typing"
)

// UnreadChanged is the data of an unread.changed event.
type UnreadChanged struct {
	Total int `json:"total"`
}

// Typing is the data of a typing event.
type Typing struct {
	ConversationID string `json:"conversationId"`
	Name           string `json:"name"`
	Active         bool   `json:"active"`
}

// events publishes UI events, including the derived ones: a conversation's
// new summary and the unread total when it changes.
type events struct {
	store *store.Store
	out   Publisher
}

// publish sends one event.
func (e *events) publish(ctx context.Context, event string, data any) {
	e.out.Publish(ctx, event, data)
}

// unreadTotal returns the unread total, or 0 when it cannot be read; the
// next successful change corrects the badge.
func (e *events) unreadTotal(ctx context.Context) int {
	total, err := e.store.UnreadTotal(ctx)
	if err != nil {
		return 0
	}

	return total
}

// conversationChanged publishes the conversation's current summary, then
// the unread total if it differs from before.
func (e *events) conversationChanged(ctx context.Context, id string, unreadBefore int) {
	conv, err := e.store.Conversation(ctx, id)
	if err != nil {
		return
	}

	e.publish(ctx, EventConversationUpdated, conv)

	if after := e.unreadTotal(ctx); after != unreadBefore {
		e.publish(ctx, EventUnreadChanged, UnreadChanged{Total: after})
	}
}
