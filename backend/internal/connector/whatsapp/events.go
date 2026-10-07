package whatsapp

// events.go registers this connector's one whatsmeow event handler for a
// run and dispatches each event to the code that handles its kind, so
// Run itself does not need to know about every event type whatsmeow can
// report: receipts (delivery and read progress for messages this
// account sent, see receipts.go), history sync and live messages (see
// history.go and live.go), typing, and the pin, archive and mute changes
// a phone makes to its own chat list. Keeping every case a single call
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
// connection status.
func (c *Connector) handleEvents(ctx context.Context, dev device, media *mediaStore, sink connector.Sink) (unregister func()) {
	return dev.onEvent(func(evt any) {
		switch e := evt.(type) {
		case *events.Receipt:
			c.receipt(ctx, sink, e)
		case *events.HistorySync:
			c.handleHistorySync(ctx, sink, dev, media, e)
		case *events.Message:
			c.handleMessage(ctx, sink, dev, media, e)
		case *events.ChatPresence:
			c.handleChatPresence(ctx, sink, e)
		case *events.Pin:
			c.handlePin(ctx, sink, e)
		case *events.Archive:
			c.handleArchive(ctx, sink, e)
		case *events.Mute:
			// Deliberately not propagated: see the decision on WhatsApp's
			// mute sync in docs/decisions.md. Listed here so the switch
			// stays the map of every event this connector has considered.
		}
	})
}
