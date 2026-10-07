package whatsapp

// handleEvents is driven through the fake device's onEvent registration,
// firing a raw event as whatsmeow would, rather than calling a handler
// directly as the other handler tests do: this is what actually proves
// the dispatcher in events.go is wired to each case, and that Run itself
// wires handleEvents to a running device.

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

func TestHandleEvents_DispatchesReceiptsToSink(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media, err := newInMemoryMediaStore(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	unregister := c.handleEvents(t.Context(), dev, media, &sink)
	defer unregister()

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-7"}); err != nil {
		t.Fatal(err)
	}

	dev.fireEvent(&events.Receipt{
		MessageSource: types.MessageSource{Chat: directPeer, Sender: directPeer},
		Type:          types.ReceiptTypeRead,
		MessageIDs:    []types.MessageID{dev.sent[0].id},
	})

	if !sink.Has("outgoing local-7  " + domain.StatusRead) {
		t.Errorf("events = %q, want local-7 read, dispatched through handleEvents", sink.Lines())
	}
}

func TestHandleEvents_IgnoresAnEventKindItDoesNotSwitchOn(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media, err := newInMemoryMediaStore(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	unregister := c.handleEvents(t.Context(), dev, media, &sink)
	defer unregister()

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-8"}); err != nil {
		t.Fatal(err)
	}

	dev.fireEvent(&events.Connected{}) // onStatus's own handler covers connection status, not this switch

	if updates := sink.Outgoing("local-8"); len(updates) != 1 {
		t.Errorf("outgoing updates = %v, want only the initial sent status", updates)
	}
}

func TestHandleEvents_UnregisterStopsFurtherDispatch(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media, err := newInMemoryMediaStore(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	unregister := c.handleEvents(t.Context(), dev, media, &sink)

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-9"}); err != nil {
		t.Fatal(err)
	}

	unregister()
	dev.fireEvent(&events.Receipt{
		MessageSource: types.MessageSource{Chat: directPeer, Sender: directPeer},
		Type:          types.ReceiptTypeRead,
		MessageIDs:    []types.MessageID{dev.sent[0].id},
	})

	if updates := sink.Outgoing("local-9"); len(updates) != 1 {
		t.Errorf("outgoing updates = %v, want no read reported after unregister", updates)
	}
}

func TestHandleEvents_DropsAMuteChangeWithoutReportingAnything(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media, err := newInMemoryMediaStore(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	unregister := c.handleEvents(t.Context(), dev, media, &sink)
	defer unregister()

	dev.fireEvent(&events.Mute{JID: types.NewJID("15551234567", types.DefaultUserServer)})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want a mute change to report nothing", sink.Lines())
	}
}

func TestRun_RoutesEventsToTheirHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		c := newTestConnector(dev)
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- c.Run(ctx, &sink) }()
		synctest.Wait()

		dev.fireEvent(&events.HistorySync{Data: &waHistorySync.HistorySync{
			Conversations: []*waHistorySync.Conversation{{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia")}},
		}})
		synctest.Wait()

		if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
			t.Errorf("events = %q, want the history sync routed to its handler", sink.Lines())
		}

		dev.fireEvent(&events.Message{
			Info: types.MessageInfo{MessageSource: types.MessageSource{
				Chat: types.NewJID("15551234567", types.DefaultUserServer), Sender: types.NewJID("15551234567", types.DefaultUserServer),
			}, ID: "M1"},
			Message: &waE2E.Message{Conversation: strPtr("hi")},
		})
		synctest.Wait()

		if !sink.Has("incoming 15551234567@s.whatsapp.net M1") {
			t.Errorf("events = %q, want the message routed to its handler", sink.Lines())
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}
