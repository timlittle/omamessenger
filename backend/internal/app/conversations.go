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
// it when there is none yet. Its title is always reported verified: the
// user deliberately chose this exact contact by its shown name from
// their own list, a different trust moment from an unsolicited incoming
// message naming itself whatever it likes (see domain.Conversation.TitleVerified
// and docs/decisions.md).
func (c *Commands) OpenConversation(ctx context.Context, accountID, contactID string) (domain.Conversation, error) {
	if accountID == "" || contactID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: accountId and contactId are required", ErrInvalidInput)
	}

	contact, err := c.store.Contact(ctx, accountID, contactID)
	if err != nil {
		return domain.Conversation{}, err
	}

	conv, created, err := c.store.EnsureConversation(ctx, domain.Conversation{
		AccountID: accountID, RemoteID: contact.RemoteID, Kind: domain.KindDirect, Title: contact.Name, TitleVerified: true,
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
// reported back as a safe message the UI can show, through pinError,
// rather than the generic internal error.
func (c *Commands) SetPinned(ctx context.Context, conversationID string, pinned bool) (domain.Conversation, error) {
	if err := c.store.SetPinned(ctx, conversationID, pinned); err != nil {
		return domain.Conversation{}, err
	}

	conv, err := c.events.publishConversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	return conv, c.pinError(ctx, conv.AccountID, pinned, c.organizer.SetPinned(ctx, conv, pinned))
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
// with a safe message naming the limit; any other failure, once the
// local pin already stands, gets the same treatment archiveError gives
// SetArchived's own service failures: a safe message naming what
// happened, logged with its safe, precise class, rather than the fixed
// internal error a request that changed nothing at all would get. Nil
// passes through unchanged, as does connector.ErrOrganizeLocalOnly
// once organizeLocalOnlyHandled has shown its own one-time notice for
// accountID instead: that is never reported as a failure at all (see
// organizenotice.go).
func (c *Commands) pinError(ctx context.Context, accountID string, pinned bool, err error) error {
	if err == nil {
		return nil
	}

	if c.organizeLocalOnlyHandled(ctx, accountID, err) {
		return nil
	}

	if errors.Is(err, connector.ErrPinLimit) {
		return fmt.Errorf("%w: this service limits how many conversations can stay pinned", ErrInvalidInput)
	}

	class := organizeFailureClass(err)
	c.logger.Printf("conversations: set pinned failed (pinned=%t, error_class=%s)", pinned, class)

	verb := "pinned"
	if !pinned {
		verb = "unpinned"
	}

	return fmt.Errorf("%w: %s here, but the service did not accept the change", ErrInvalidInput, verb)
}

// SetArchived files a conversation away, or brings it back, then tells
// its service. Archiving a conversation this account had pinned also
// unpins it here first, for any service whose own archiving does the
// same (see ArchiveUnpinner): otherwise the list would show a
// conversation both pinned and archived, a combination the real
// service never has, and that stale local pin would stay stuck forever
// once the service call below fails, since nothing else would ever
// come along to correct it (see docs/decisions.md). That pin is
// remembered first (RememberPinnedBeforeArchive) so unarchiving can
// give it back: a person archiving, then later bringing a chat back,
// does not expect the pin they had on it to be gone for good just
// because the service's own archive always unpins too. The local
// change always stands even if the service call fails; that failure is
// reported back as a safe message the UI can show, through
// archiveError, rather than the generic internal error.
func (c *Commands) SetArchived(ctx context.Context, conversationID string, archived bool) (domain.Conversation, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}

	if archived {
		if err := c.unpinForArchive(ctx, conv); err != nil {
			return domain.Conversation{}, err
		}
	}

	if err := c.store.SetArchived(ctx, conversationID, archived); err != nil {
		return domain.Conversation{}, err
	}

	restoredPin, err := c.restorePinForUnarchive(ctx, conversationID, archived)
	if err != nil {
		return domain.Conversation{}, err
	}

	conv, err = c.events.publishConversation(ctx, conversationID)
	if err != nil {
		return conv, err
	}

	serviceErr := c.organizer.SetArchived(ctx, conv, archived)
	if restoredPin && serviceErr == nil {
		c.rePinAfterUnarchive(ctx, conv)
	}

	return conv, c.archiveError(ctx, conv.AccountID, archived, serviceErr)
}

// unpinForArchive clears conv's local pin before SetArchived archives
// it, for a service whose own archive always unpins too (see
// ArchiveUnpinner): otherwise the list would show a conversation both
// pinned and archived, a combination the real service never has. The
// pin is remembered first (RememberPinnedBeforeArchive) so unarchiving
// can restore it later; a service that does not require this is left
// alone entirely.
func (c *Commands) unpinForArchive(ctx context.Context, conv domain.Conversation) error {
	if !c.archiveUnpinner.ArchiveUnpins(conv.AccountID) {
		return nil
	}

	if err := c.store.RememberPinnedBeforeArchive(ctx, conv.ID); err != nil {
		return err
	}

	return c.store.SetPinned(ctx, conv.ID, false)
}

// restorePinForUnarchive gives conversationID back the pin archiving
// took away (see RestorePinnedFromArchive), reporting whether it
// actually restored one so SetArchived knows whether to also ask the
// service to re-pin it; a no-op, reporting false, while archiving
// rather than unarchiving.
func (c *Commands) restorePinForUnarchive(ctx context.Context, conversationID string, archived bool) (bool, error) {
	if archived {
		return false, nil
	}

	return c.store.RestorePinnedFromArchive(ctx, conversationID)
}

// rePinAfterUnarchive asks conv's service to re-pin it, once its own
// unarchive already succeeded, so the pin SetArchived already restored
// locally (see RestorePinnedFromArchive) agrees with the service too.
// Best effort, like MarkRead's own chat-read mutation: the local pin
// stands either way, and a later resync corrects any miss, so a
// failure here is only logged, with its safe, precise class, never
// returned to the caller.
func (c *Commands) rePinAfterUnarchive(ctx context.Context, conv domain.Conversation) {
	err := c.organizer.SetPinned(ctx, conv, true)
	if err == nil || c.organizeLocalOnlyHandled(ctx, conv.AccountID, err) {
		return
	}

	c.logger.Printf("conversations: re-pin after unarchive failed (error_class=%s)", organizeFailureClass(err))
}

// archiveError turns a service failure that followed SetArchived's own
// already-applied local change into ErrInvalidInput with a safe
// message the UI can show, after logging the failure's coarse, safe
// class to stderr: unlike a request that never touched the store, the
// user already has the result they asked for here, so even a failure
// this package cannot name more precisely is safe to describe in those
// terms, rather than the fixed internal error a request that changed
// nothing at all would get. Nil passes through unchanged, as does
// connector.ErrOrganizeLocalOnly once organizeLocalOnlyHandled has
// shown its own one-time notice for accountID instead (see pinError's
// own doc comment).
func (c *Commands) archiveError(ctx context.Context, accountID string, archived bool, err error) error {
	if err == nil {
		return nil
	}

	if c.organizeLocalOnlyHandled(ctx, accountID, err) {
		return nil
	}

	class := organizeFailureClass(err)
	c.logger.Printf("conversations: set archived failed (archived=%t, error_class=%s)", archived, class)

	verb := "archived"
	if !archived {
		verb = "unarchived"
	}

	return fmt.Errorf("%w: %s here, but the service did not accept the change", ErrInvalidInput, verb)
}

// organizeFailureClass categorizes why a pin, archive or mark-read
// change's service call failed, safe to log: "timeout" for a context
// that ran out, "sync" when a connector reports its own local copy of
// whatever state the service needs was not ready yet
// (connector.ErrOrganizeSyncIncomplete, such as WhatsApp's app-state
// sync never finishing after pairing), "rejected" when a connector
// reports the service explicitly refused the change
// (connector.ErrOrganizeRejected), "local" when a connector reports
// the change could never be synced at all right now
// (connector.ErrOrganizeLocalOnly) - every caller of this is expected
// to have already handled that one through organizeLocalOnlyHandled
// and never reach here with it, this class exists only so a caller
// that forgets still logs something sensible rather than "other" -
// and "other" for anything else, such as a plain network failure.
// Never the error's own text, which a connector does not guarantee
// free of an id or similar detail.
func organizeFailureClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, connector.ErrOrganizeSyncIncomplete):
		return "sync"
	case errors.Is(err, connector.ErrOrganizeRejected):
		return "rejected"
	case errors.Is(err, connector.ErrOrganizeLocalOnly):
		return "local"
	default:
		return "other"
	}
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
