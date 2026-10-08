package whatsapp

// organize.go keeps a conversation's pinned and archived state in step
// with WhatsApp, treating whatsmeow's own chat settings store (see
// device.chatSettings) as the single source of truth: this connector
// never caches pinned or archived itself, so there is nothing of its
// own that a stale sync could ever revert.
//
// SetPinned and SetArchived send an app-state patch for the change,
// the same way WhatsApp's own app would, then read the chat settings
// straight back and report those. That read-back already sees the
// change: whatsmeow's SendAppState fetches and applies the server's
// own patches to its local store synchronously, before it returns, so
// by the time the patch send succeeds here, the store already agrees
// (see docs/decisions.md). Only the events whatsmeow fires for other
// listeners are dispatched afterwards, in the background, which is why
// handlePin and handleArchive, below, do exactly the same read-and-report
// rather than trusting the one field their own event names: a live
// Pin or Archive is just WhatsApp's own notice that something changed,
// never the new value itself. handleMarkChatAsRead is the equivalent
// notice for MarkRead's own patch (see markread.go), which carries no
// state of its own to read back.
//
// A history sync's own, separate snapshot of pinned and archived (see
// history.go) is likewise never consulted: syncConversation reads the
// same chat settings store instead, which is what stops a resync from
// ever reverting a pin or archive this process, or the phone, just
// made, and lets a pin that arrived before a conversation existed
// apply itself the moment that conversation is finally reported.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// organizeTimeout bounds how long SetPinned and SetArchived wait for
// WhatsApp to accept an app-state patch, so an account with network
// trouble cannot leave a caller waiting forever.
const organizeTimeout = 30 * time.Second

// pinLimit is the most conversations WhatsApp's own app lets stay
// pinned at once. WhatsApp enforces this itself rather than returning a
// distinct, structured error from the server, so this connector matches
// that behaviour by counting whatsmeow's own chat settings, instead of
// trying to tell the refusal apart from any other rejected app-state
// patch by its text.
const pinLimit = 3

// SetPinned pins or unpins conv with WhatsApp, refusing with
// connector.ErrPinLimit a pin that would push past WhatsApp's own limit
// before ever asking the server.
func (c *Connector) SetPinned(ctx context.Context, conv domain.Conversation, pinned bool) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: set pinned: %w", err)
	}
	if pinned && c.pinnedCount(ctx, dev, conv.RemoteID) >= pinLimit {
		return fmt.Errorf("whatsapp: set pinned: %w", connector.ErrPinLimit)
	}

	patchCtx, cancel := context.WithTimeout(ctx, organizeTimeout)
	defer cancel()
	if err := dev.sendAppState(patchCtx, appstate.BuildPin(jid, pinned)); err != nil {
		return fmt.Errorf("whatsapp: set pinned: %w", err)
	}

	return c.reportOrganized(ctx, sink, dev, jid, conv.RemoteID)
}

// SetArchived files conv away, or brings it back, with WhatsApp.
// Archiving a chat also unpins it there: appstate.BuildArchive already
// bundles the matching unpin mutation into the same patch, so the
// settings this reads back afterwards already reflect that, with
// nothing for this connector to compute itself.
func (c *Connector) SetArchived(ctx context.Context, conv domain.Conversation, archived bool) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: set archived: %w", err)
	}

	patchCtx, cancel := context.WithTimeout(ctx, organizeTimeout)
	defer cancel()
	if err := dev.sendAppState(patchCtx, appstate.BuildArchive(jid, archived, time.Time{}, nil)); err != nil {
		return fmt.Errorf("whatsapp: set archived: %w", err)
	}

	return c.reportOrganized(ctx, sink, dev, jid, conv.RemoteID)
}

// reportOrganized reports remoteID's current pinned and archived
// state, read straight from whatsmeow's own chat settings store for
// jid (see device.chatSettings): a read failure here should not arise,
// since the patch that led here already succeeded, but is still
// reported rather than silently guessed at.
func (c *Connector) reportOrganized(ctx context.Context, sink connector.Sink, dev device, jid types.JID, remoteID string) error {
	pinned, archived, err := dev.chatSettings(ctx, jid)
	if err != nil {
		return fmt.Errorf("whatsapp: chat settings: %w", err)
	}

	sink.Organized(ctx, c.account.ID, remoteID, pinned, archived)

	return nil
}

// pinnedCount is how many conversations other than excludeRemoteID,
// among the ones this connector has ever reported (see
// Connector.chatKinds), WhatsApp's own chat settings currently have
// pinned. A chat WhatsApp already has pinned before this connector has
// ever reported it is not countable yet, the same limit an in-memory
// cache of this connector's own would have had too; a later sync or
// message naturally brings it into chatKinds and so into this count.
func (c *Connector) pinnedCount(ctx context.Context, dev device, excludeRemoteID string) int {
	n := 0
	for _, remote := range c.knownChatIDs() {
		if remote == excludeRemoteID {
			continue
		}

		jid, err := jidFromRemoteID(remote)
		if err != nil {
			continue
		}
		if pinned, _, err := dev.chatSettings(ctx, jid); err == nil && pinned {
			n++
		}
	}

	return n
}

// reportLiveOrganize reports jid's chat pinned and archived exactly as
// WhatsApp's own chat settings store currently has it, the state a
// live Pin or Archive event's own arrival already means was applied
// there (see this file's own doc comment); handlePin and handleArchive
// share this, since a read failure here logs the same way and neither
// has anything else of its own to do with one.
func (c *Connector) reportLiveOrganize(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, jid types.JID) {
	pinned, archived, err := dev.chatSettings(ctx, jid)
	if err != nil {
		logOrganizeReadFailed(err)
		return
	}

	sink.Organized(ctx, c.account.ID, chatID(ctx, dev, media, jid), pinned, archived)
}

// handlePin reports a chat pinned or unpinned from the phone.
func (c *Connector) handlePin(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Pin) {
	c.reportLiveOrganize(ctx, sink, dev, media, e.JID)
}

// handleArchive reports a chat archived or unarchived from the phone.
func (c *Connector) handleArchive(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Archive) {
	c.reportLiveOrganize(ctx, sink, dev, media, e.JID)
}

// handleMarkChatAsRead reports a chat WhatsApp's own app-state sync says
// was marked read from the phone or another linked device: the
// counterpart of MarkRead's own chat-level patch (see markread.go),
// arriving the other way. A chat marked unread this way is left alone:
// nothing in this connector tracks how many messages that would put
// back, and WhatsApp's own unread count, synced separately, corrects
// it regardless.
func (c *Connector) handleMarkChatAsRead(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.MarkChatAsRead) {
	if !e.Action.GetRead() {
		return
	}

	sink.Unread(ctx, c.account.ID, chatID(ctx, dev, media, e.JID), 0)
}
