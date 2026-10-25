package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// pinnedParams pins or unpins a conversation.
type pinnedParams struct {
	ConversationID string `json:"conversationId"`
	Pinned         bool   `json:"pinned"`
}

// conversationsSetPinned pins or unpins a conversation.
func conversationsSetPinned(c *app.Commands) func(context.Context, pinnedParams) (any, error) {
	return func(ctx context.Context, p pinnedParams) (any, error) {
		return c.SetPinned(ctx, p.ConversationID, p.Pinned)
	}
}

// movePinParams names the pinned conversation to move, and which way:
// "up" or "down".
type movePinParams struct {
	ConversationID string `json:"conversationId"`
	Direction      string `json:"direction"`
}

// conversationsMovePin moves a pinned conversation one place up or down
// among the other pinned conversations.
func conversationsMovePin(c *app.Commands) func(context.Context, movePinParams) (any, error) {
	return func(ctx context.Context, p movePinParams) (any, error) {
		return c.MovePin(ctx, p.ConversationID, p.Direction)
	}
}
