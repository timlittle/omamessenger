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
// service, unless the user has turned read receipts off: incognito mode
// clears the badge locally but never reports it read, so the service,
// and any other device signed into the same account, keep showing the
// chat unread. The service hears the count this conversation actually
// had before clearing it: a connector such as WhatsApp's needs that
// number to know how many of a conversation's newest incoming messages
// to send a read receipt for, and asking the store only after the local
// clear would always report zero, turning every such call into a silent
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

	settings, _, _ := c.ui.snapshot()
	if !settings.ReadReceipts {
		return nil
	}

	return c.dispatcher.MarkRead(ctx, conv)
}

// MarkUnread flags a conversation unread again, for the command
// palette's "Mark as unread": entirely local to OmaMessenger, like
// SetHidden and SetReminder, since there is nothing useful to tell a
// service whose own unread count is what Unread already syncs back. It
// gives the conversation a count of 1, the same generic placeholder
// every other inbox shows for "something here again", and does nothing
// when it already has an unread count of its own, so this can never
// lower a real count the service reported.
func (c *Commands) MarkUnread(ctx context.Context, conversationID string) (domain.Conversation, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}

	if conv.Unread > 0 {
		return conv, nil
	}

	before := c.events.unreadTotal(ctx)
	if _, err := c.store.SetUnread(ctx, conversationID, 1); err != nil {
		return domain.Conversation{}, err
	}

	c.events.conversationChanged(ctx, conversationID, before)

	return c.store.Conversation(ctx, conversationID)
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

	conv, err := c.events.publishConversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	return conv, pinError(c.organizer.SetPinned(ctx, conv, pinned))
}

// MovePin moves a pinned conversation one place up or down among the
// other pinned conversations, through the store's own adjacent swap,
// then publishes both conversations' new summaries so every open view
// agrees: unlike SetPinned, nothing here ever reaches a service, since
// a service's own notion of pin order (WhatsApp's own app order, say) is
// never something this computer's local reordering is meant to drive.
// direction must be "up" or "down"; anything else is ErrInvalidInput.
func (c *Commands) MovePin(ctx context.Context, conversationID, direction string) (domain.Conversation, error) {
	up, err := parsePinDirection(direction)
	if err != nil {
		return domain.Conversation{}, err
	}

	neighborID, err := c.store.MovePin(ctx, conversationID, up)
	if err != nil {
		return domain.Conversation{}, err
	}

	if neighborID != "" {
		if _, err := c.events.publishConversation(ctx, neighborID); err != nil {
			return domain.Conversation{}, err
		}
	}

	return c.events.publishConversation(ctx, conversationID)
}

// parsePinDirection turns the protocol's "up" or "down" into the up bool
// store.MovePin takes, rejecting anything else as invalid input safe to
// show the user.
func parsePinDirection(direction string) (bool, error) {
	switch direction {
	case "up":
		return true, nil
	case "down":
		return false, nil
	default:
		return false, fmt.Errorf(`%w: direction must be "up" or "down"`, ErrInvalidInput)
	}
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

// SetArchived files a conversation away, or brings it back, then tells
// its service. Archiving a conversation this account had pinned also
// unpins it here first, for any service whose own archiving does the
// same (see ArchiveUnpinner): otherwise the list would show a
// conversation both pinned and archived, a combination the real
// service never has, and that stale local pin would stay stuck forever
// once the service call below fails, since nothing else would ever
// come along to correct it (see docs/decisions.md). The local change
// always stands even if the service call fails; that failure is
// reported back as a safe message the UI can show, through
// archiveError, rather than the generic internal error.
func (c *Commands) SetArchived(ctx context.Context, conversationID string, archived bool) (domain.Conversation, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}

	if archived && c.archiveUnpinner.ArchiveUnpins(conv.AccountID) {
		if err := c.store.SetPinned(ctx, conversationID, false); err != nil {
			return domain.Conversation{}, err
		}
	}

	if err := c.store.SetArchived(ctx, conversationID, archived); err != nil {
		return domain.Conversation{}, err
	}

	conv, err = c.events.publishConversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	return conv, c.archiveError(archived, c.organizer.SetArchived(ctx, conv, archived))
}

// archiveError turns a service failure that followed SetArchived's own
// already-applied local change into ErrInvalidInput with a safe
// message the UI can show, after logging the failure's coarse, safe
// class to stderr: unlike a request that never touched the store, the
// user already has the result they asked for here, so even a failure
// this package cannot name more precisely is safe to describe in those
// terms, rather than the fixed internal error a request that changed
// nothing at all would get. Nil passes through unchanged.
func (c *Commands) archiveError(archived bool, err error) error {
	if err == nil {
		return nil
	}

	class := "other"
	if errors.Is(err, context.DeadlineExceeded) {
		class = "timeout"
	}
	c.logger.Printf("conversations: set archived failed (archived=%t, error_class=%s)", archived, class)

	verb := "archived"
	if !archived {
		verb = "unarchived"
	}

	return fmt.Errorf("%w: %s here, but the service did not accept the change", ErrInvalidInput, verb)
}

// SetReminder snoozes a conversation until at (Unix milliseconds), or
// clears its reminder when at is 0, then wakes the reminder scheduler so
// it recomputes when to wake next, instead of only noticing on whatever
// schedule it already had. Like hidden, this is local to OmaMessenger
// only: it is never reported to the service.
func (c *Commands) SetReminder(ctx context.Context, conversationID string, at int64) (domain.Conversation, error) {
	if err := c.store.SetReminder(ctx, conversationID, at); err != nil {
		return domain.Conversation{}, err
	}

	conv, err := c.events.publishConversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	c.reminders.notifyChanged()

	return conv, nil
}

// SetHidden hides or unhides a conversation from the standard list. Unlike
// mute, pin and archive, this is local to OmaMessenger only: it is never
// reported to the service, so no connector call follows.
func (c *Commands) SetHidden(ctx context.Context, conversationID string, hidden bool) (domain.Conversation, error) {
	if err := c.store.SetHidden(ctx, conversationID, hidden); err != nil {
		return domain.Conversation{}, err
	}

	return c.events.publishConversation(ctx, conversationID)
}
