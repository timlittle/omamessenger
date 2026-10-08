package whatsapp

// organize.go keeps a conversation's pinned, archived and read state in
// step with WhatsApp in both directions: Connector.SetPinned and
// SetArchived (MarkRead's own chat-level patch lives in markread.go
// instead, next to the per-message receipts it sends alongside it) each
// send an app-state patch for the change, the same way WhatsApp's own
// app would, then update the state this connector caches for the
// conversation (see connector.go's organizeState) so a later echo of
// the same change arriving from the phone (handlePin and handleArchive,
// below) confirms it rather than reverting it. handleMarkChatAsRead is
// that same echo for MarkRead's own patch, arriving the other way.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"
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
// that behaviour using the pinned state it already caches, instead of
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
	if pinned && c.pinnedCount(conv.RemoteID) >= pinLimit {
		return fmt.Errorf("whatsapp: set pinned: %w", connector.ErrPinLimit)
	}

	patchCtx, cancel := context.WithTimeout(ctx, organizeTimeout)
	defer cancel()
	if err := dev.sendAppState(patchCtx, appstate.BuildPin(jid, pinned)); err != nil {
		return fmt.Errorf("whatsapp: set pinned: %w", err)
	}

	state := c.setOrganized(conv.RemoteID, &pinned, nil)
	c.markLocalOrganize(conv.RemoteID)
	sink.Organized(ctx, c.account.ID, conv.RemoteID, state.pinned, state.archived)

	return nil
}

// SetArchived files conv away, or brings it back, with WhatsApp.
// Archiving a chat also unpins it there, so a successful archive clears
// the cached pin too, matching WhatsApp's own behaviour.
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

	var unpinned *bool
	if archived {
		no := false
		unpinned = &no
	}
	state := c.setOrganized(conv.RemoteID, unpinned, &archived)
	c.markLocalOrganize(conv.RemoteID)
	sink.Organized(ctx, c.account.ID, conv.RemoteID, state.pinned, state.archived)

	return nil
}

// pinnedCount returns how many conversations other than excludeRemoteID
// this connector last knew to be pinned, so re-pinning one already
// pinned never counts against the limit.
func (c *Connector) pinnedCount(excludeRemoteID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for remote, state := range c.organize {
		if remote != excludeRemoteID && state.pinned {
			n++
		}
	}

	return n
}

// handlePin reports a chat pinned or unpinned from the phone, merging it
// with whichever archived state this connector last knew for it, and
// marks pinned as confirmed by app state so a later history sync's own
// snapshot can never revert it (see setOrganizedFromAppState).
func (c *Connector) handlePin(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Pin) {
	remote := chatID(ctx, dev, media, e.JID)
	pinned := e.Action.GetPinned()

	c.clearLocalOrganize(remote) // a live echo is WhatsApp's own current state, always trusted over a pending local change
	state := c.setOrganizedFromAppState(remote, &pinned, nil)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
}

// handleArchive reports a chat archived or unarchived from the phone,
// merging it with whichever pinned state this connector last knew for
// it, and marks archived as confirmed by app state so a later history
// sync's own snapshot can never revert it (see setOrganizedFromAppState).
func (c *Connector) handleArchive(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Archive) {
	remote := chatID(ctx, dev, media, e.JID)
	archived := e.Action.GetArchived()

	c.clearLocalOrganize(remote) // a live echo is WhatsApp's own current state, always trusted over a pending local change
	state := c.setOrganizedFromAppState(remote, nil, &archived)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
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
