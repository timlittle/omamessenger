package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Vote casts the signed-in user's choice in a message's poll, then
// tells its service so the vote is not just local. A connector that
// does not support polls, a message with no poll, or a closed poll
// reports a safe message the UI can show, not an internal error.
func (c *Commands) Vote(ctx context.Context, messageID string, optionIDs []string) (domain.Message, error) {
	if strings.TrimSpace(messageID) == "" {
		return domain.Message{}, fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}
	if len(optionIDs) == 0 {
		return domain.Message{}, fmt.Errorf("%w: at least one option is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return domain.Message{}, err
	}

	if m.RemoteID == "" {
		return domain.Message{}, fmt.Errorf("%w: this message cannot be voted on yet", ErrInvalidInput)
	}

	if err := pollVotable(m.Media); err != nil {
		return domain.Message{}, err
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}

	if err := c.voter.Vote(ctx, conv, m.RemoteID, optionIDs); err != nil {
		return domain.Message{}, voteError(err)
	}

	return c.store.Message(ctx, messageID)
}

// pollVotable reports whether media is a poll still open for voting, or
// an ErrInvalidInput explaining why not.
func pollVotable(media *domain.Media) error {
	if media == nil || media.Kind != domain.MediaPoll || media.Poll == nil {
		return fmt.Errorf("%w: this message has no poll", ErrInvalidInput)
	}

	if media.Poll.Closed {
		return fmt.Errorf("%w: this poll is closed", ErrInvalidInput)
	}

	return nil
}

// voteError turns a connector failure the user can fix into
// ErrInvalidInput with a safe message; anything else, such as a
// network failure, is returned unchanged so it becomes a fixed internal
// error.
func voteError(err error) error {
	if errors.Is(err, connector.ErrNoVoter) {
		return fmt.Errorf("%w: voting is not supported here", ErrInvalidInput)
	}

	return err
}
