package whatsapp

// dispatch is the one seam between whatsmeow's events and this
// package's handlers: Run registers it once with device.onEvent, and it
// routes each event by type into history.go (the one-off history sync)
// or live.go (everything that keeps arriving while connected). Lane B's
// outgoing send progress adds its own case here, for events.Receipt
// about messages this account sent; keeping each case to one call keeps
// that addition a one-line change.

import (
	"context"

	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// dispatch routes one whatsmeow event to the handler for its kind,
// ignoring any event none of them act on.
func (c *Connector) dispatch(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, evt any) {
	switch e := evt.(type) {
	case *events.HistorySync:
		c.handleHistorySync(ctx, sink, dev, media, e)
	case *events.Message:
		c.handleMessage(ctx, sink, dev, media, e)
	case *events.ChatPresence:
		c.handleChatPresence(ctx, sink, e)
	case *events.Receipt:
		c.handleReceipt(ctx, sink, e)
	case *events.Pin:
		c.handlePin(ctx, sink, e)
	case *events.Archive:
		c.handleArchive(ctx, sink, e)
	case *events.Mute:
		// Deliberately not propagated: see the decision on WhatsApp's
		// mute sync in docs/decisions.md. Listed here so the switch
		// stays the map of every event this connector has considered.
	}
}
