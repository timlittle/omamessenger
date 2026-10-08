package whatsapp

// events.go registers this connector's one whatsmeow event handler for a
// run and dispatches each event to the code that handles its kind, so
// Run itself does not need to know about every event type whatsmeow can
// report: receipts (delivery and read progress for messages this
// account sent, see receipts.go), history sync and live messages (see
// history.go and live.go), typing, the pin, archive and mute changes a
// phone makes to its own chat list, and a message deleted "for me" on
// another linked device (see delete.go). Keeping every case a single call
// into another file is what keeps this switch easy to extend without
// conflict: a new kind of event is a new case, never a change to how
// Run wires this up.

import (
	"context"

	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// handleEvents registers dev's one event handler for this run and
// returns a func that unregisters it, mirroring dev.onStatus's shape for
// connection status. Events this connector only ever re-titles a chat
// from, or deliberately ignores, are left to handleNameEvent, so this
// switch's own complexity does not grow with every later addition there.
func (c *Connector) handleEvents(ctx context.Context, dev device, media *mediaStore, sink connector.Sink) (unregister func()) {
	return dev.onEvent(func(evt any) {
		switch e := evt.(type) {
		case *events.Receipt:
			// Lane B (delivery and read progress for a message this
			// account sent) and lane A (this account's own read, synced
			// from another of its devices) each act on the field of e
			// that is theirs and ignore the rest, so both always run:
			// see receipt's and handleReceipt's own doc comments.
			c.receipt(ctx, sink, dev, e)
			c.handleReceipt(ctx, sink, dev, e)
		case *events.HistorySync:
			c.handleHistorySync(ctx, sink, dev, media, e)
		case *events.Message:
			c.handleMessage(ctx, sink, dev, media, e)
		case *events.UndecryptableMessage:
			c.handleUndecryptable(ctx, sink, dev, e)
		case *events.ChatPresence:
			c.handleChatPresence(ctx, sink, dev, e)
		case *events.Pin:
			c.handlePin(ctx, sink, dev, e)
		case *events.Archive:
			c.handleArchive(ctx, sink, dev, e)
		case *events.DeleteForMe:
			c.handleDeleteForMe(ctx, sink, dev, e)
		default:
			c.handleNameEvent(ctx, sink, dev, evt)
		}
	})
}

// handleNameEvent dispatches the events that can only retitle an
// already-known chat (see contacts.go), the mute changes this
// connector deliberately does not propagate, and the primary phone's
// answer to a media retry request (see retry.go): none of these need
// their own case in handleEvents' own switch, which already sits at
// this linter's complexity limit.
func (c *Connector) handleNameEvent(ctx context.Context, sink connector.Sink, dev device, evt any) {
	switch e := evt.(type) {
	case *events.Contact:
		c.handleContactUpdate(ctx, sink, dev, e)
	case *events.PushName:
		c.handlePushNameUpdate(ctx, sink, dev, e)
	case *events.AppStateSyncComplete:
		c.handleAppStateSyncComplete(ctx, sink, dev, e)
	case *events.MediaRetry:
		c.deliverRetry(e)
	case *events.Mute:
		// Deliberately not propagated: see the decision on WhatsApp's
		// mute sync in docs/decisions.md. Listed here so this switch
		// stays the map of every such event this connector has
		// considered.
	}
}
