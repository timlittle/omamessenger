package app

// edit.go implements the UI's request to edit one of this account's own
// sent text messages: the store is updated first, optimistically, then
// the conversation's service is told, through the Editor the helper
// wired up (see connector.Manager.EditMessage). A refusal reverts the
// store back to the message's original text; the service's own later
// echo of a successful edit (see Ingest.Edited) updates the very same
// row by remote id, so it lands as a no-op rather than a second change.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// EditMessage changes the text of a message this account already sent.
// Only an outgoing text message that has already reached the service
// (it carries a remote id) can be edited; a message still attaching a
// photo, video or file, still pending, or belonging to another
// conversation, fails with ErrInvalidInput, as does a connector's own
// refusal - the message is not this account's, or past its service's
// time limit to edit.
func (c *Commands) EditMessage(ctx context.Context, conversationID, messageID, text string) (domain.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Message{}, fmt.Errorf("%w: text is required", ErrInvalidInput)
	}
	if utf8.RuneCountInString(text) > domain.MaxTextLength {
		return domain.Message{}, fmt.Errorf("%w: %w", ErrInvalidInput, domain.ErrTextTooLong)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return domain.Message{}, err
	}
	if m.ConversationID != conversationID {
		return domain.Message{}, fmt.Errorf("%w: messageId must belong to the conversation", ErrInvalidInput)
	}
	if !editable(m) {
		return domain.Message{}, fmt.Errorf("%w: this message cannot be edited", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Message{}, err
	}

	updated, _, err := c.store.EditMessage(ctx, conv.ID, m.RemoteID, editOf(m, text))
	if err != nil {
		return domain.Message{}, err
	}
	c.events.publish(ctx, EventMessageUpdated, updated)

	if err := c.editor.EditMessage(ctx, conv, m.RemoteID, text, m.Mentions); err != nil {
		c.revertEdit(ctx, conv, m)

		return domain.Message{}, editError(err)
	}

	return updated, nil
}

// editable reports whether m is something EditMessage can change: this
// account's own text message, with no attachment, that has already
// reached the service.
func editable(m domain.Message) bool {
	return m.Outgoing && m.Media == nil && m.RemoteID != ""
}

// editOf is the store update EditMessage applies for text, keeping m's
// media, reactions, mentions and formatting exactly as they were:
// editing changes only the words.
func editOf(m domain.Message, text string) store.MessageEdit {
	return store.MessageEdit{
		Text: text, Media: m.Media, Reactions: m.Reactions, Mentions: m.Mentions, MentionsMe: m.MentionsMe, Formatting: m.Formatting,
	}
}

// revertEdit undoes the optimistic update EditMessage already applied,
// once the service has refused it, restoring original's own text and
// publishing the restored message. Best effort: a failure here only
// means the UI keeps showing the rejected text until the next refresh,
// which matters far less than the refusal itself already being
// reported back to the caller.
func (c *Commands) revertEdit(ctx context.Context, conv domain.Conversation, original domain.Message) {
	reverted, _, err := c.store.EditMessage(ctx, conv.ID, original.RemoteID, editOf(original, original.Text))
	if err != nil {
		return
	}

	c.events.publish(ctx, EventMessageUpdated, reverted)
}

// editError turns a connector's refusal the user can act on into
// ErrInvalidInput with a safe message; anything else, such as a network
// failure, is returned unchanged so it becomes a fixed internal error.
func editError(err error) error {
	if errors.Is(err, connector.ErrNoEditor) || errors.Is(err, connector.ErrEditUnsupported) {
		return fmt.Errorf("%w: this message cannot be edited", ErrInvalidInput)
	}

	return err
}
