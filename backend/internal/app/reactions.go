package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// React sets or clears the user's own reaction to a message, then tells
// its service so the change is not just local; emoji "" clears it. A
// connector that does not support reactions reports a safe message the
// UI can show, not an internal error.
func (c *Commands) React(ctx context.Context, messageID, emoji string) (domain.Message, error) {
	if strings.TrimSpace(messageID) == "" {
		return domain.Message{}, fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return domain.Message{}, err
	}

	if m.RemoteID == "" {
		return domain.Message{}, fmt.Errorf("%w: this message cannot be reacted to yet", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}

	if err := c.reactor.React(ctx, conv, m.RemoteID, emoji); err != nil {
		return domain.Message{}, reactError(err)
	}

	return c.store.Message(ctx, messageID)
}

// reactError turns a connector failure the user can fix into
// ErrInvalidInput with a safe message; anything else, such as a network
// failure, is returned unchanged so it becomes a fixed internal error.
func reactError(err error) error {
	if errors.Is(err, connector.ErrNoReactions) {
		return fmt.Errorf("%w: reactions are not supported here", ErrInvalidInput)
	}

	return err
}
