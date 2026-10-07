package whatsapp

// events.go registers this connector's one whatsmeow event handler for a
// run and dispatches each event to the code that handles its kind, so
// Run itself does not need to know about every event type whatsmeow can
// report. Live incoming messages and history sync add their own cases to
// the switch below; this file adds only the receipt case, which reports
// delivery and read progress for the messages this account sent (see
// receipts.go).

import (
	"context"

	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// handleEvents registers dev's one event handler for this run and
// returns a func that unregisters it, mirroring dev.onStatus's shape for
// connection status.
func (c *Connector) handleEvents(ctx context.Context, dev device, sink connector.Sink) (unregister func()) {
	return dev.onEvent(func(evt any) {
		switch e := evt.(type) {
		case *events.Receipt:
			c.receipt(ctx, sink, e)
		}
	})
}
