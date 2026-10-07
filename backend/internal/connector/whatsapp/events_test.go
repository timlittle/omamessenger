package whatsapp

// handleEvents is driven through the fake device's onEvent registration,
// firing a raw event as whatsmeow would, rather than calling receipt
// directly as the other receipt tests do: this is what actually proves
// the dispatcher in events.go is wired to the receipt case.

import (
	"testing"

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

	unregister := c.handleEvents(t.Context(), dev, &sink)
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

	unregister := c.handleEvents(t.Context(), dev, &sink)
	defer unregister()

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-8"}); err != nil {
		t.Fatal(err)
	}

	dev.fireEvent(&events.Connected{}) // live.go and history.go add their own cases for events like this later

	if updates := sink.Outgoing("local-8"); len(updates) != 1 {
		t.Errorf("outgoing updates = %v, want only the initial sent status", updates)
	}
}

func TestHandleEvents_UnregisterStopsFurtherDispatch(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	unregister := c.handleEvents(t.Context(), dev, &sink)

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
