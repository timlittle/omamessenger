package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// contactsBlock blocks a direct conversation's contact.
func contactsBlock(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.BlockContact(ctx, p.ConversationID, true)
	}
}

// contactsUnblock unblocks a direct conversation's contact.
func contactsUnblock(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.BlockContact(ctx, p.ConversationID, false)
	}
}

// conversationsReportSpam reports a direct conversation's contact as
// spam, blocking the contact and hiding the conversation.
func conversationsReportSpam(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.ReportSpam(ctx, p.ConversationID)
	}
}
