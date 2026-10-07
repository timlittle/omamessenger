package whatsapp

// dispatch is unexported, and Run is the only caller that wires it to a
// device's events, so these tests drive it the way Run does: through a
// fake device's onEvent handler, inside a running connector.

import (
	"context"
	"testing"
	"testing/synctest"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRun_RoutesEventsToTheirHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		c := newTestConnector(t, dev)
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- c.Run(ctx, &sink) }()
		synctest.Wait()

		dev.fire(&events.HistorySync{Data: &waHistorySync.HistorySync{
			Conversations: []*waHistorySync.Conversation{{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia")}},
		}})
		synctest.Wait()

		if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
			t.Errorf("events = %q, want the history sync routed to its handler", sink.Lines())
		}

		dev.fire(&events.Message{
			Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("15551234567", types.DefaultUserServer), Sender: types.NewJID("15551234567", types.DefaultUserServer)}, ID: "M1"},
			Message: &waE2E.Message{Conversation: strPtr("hi")},
		})
		synctest.Wait()

		if !sink.Has("incoming 15551234567@s.whatsapp.net M1") {
			t.Errorf("events = %q, want the message routed to its handler", sink.Lines())
		}

		// An event no handler acts on, and the pairing status events
		// onStatus already covers, must never panic the dispatcher.
		dev.fire(&events.QR{})
		synctest.Wait()

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestDispatch_MuteIsReceivedButNotPropagated(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	c.dispatch(t.Context(), &sink, dev, media, &events.Mute{JID: types.NewJID("15551234567", types.DefaultUserServer)})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want a mute change to report nothing", sink.Lines())
	}
}
