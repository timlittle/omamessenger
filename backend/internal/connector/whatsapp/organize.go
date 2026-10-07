package whatsapp

// organize.go keeps a conversation's pinned and archived state in step
// with WhatsApp: Connector.SetPinned and SetArchived each send an
// app-state patch for the change, the same way WhatsApp's own app would,
// then update the state this connector caches for the conversation (see
// connector.go's organizeState) so a later echo of the same change
// arriving from the phone (see live.go's handlePin and handleArchive)
// confirms it rather than reverting it.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"

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
