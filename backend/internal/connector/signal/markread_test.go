package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMarkRead_SendsReceiptPerSender checks that MarkRead sends one
// read receipt per distinct sender among conv.Unread's own newest
// messages, carrying each sender's own timestamps.
func TestMarkRead_SendsReceiptPerSender(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	senderA := uuid.New()
	senderB := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: "conv-1", Unread: 3}
	idA := messageRemoteID(senderA, 1700000000000)
	idB := messageRemoteID(senderB, 1700000000001)
	for _, id := range []string{idA, idB} {
		if err := media.putMessageMeta(t.Context(), conv.RemoteID, id, "guid-"+id, nil); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}

	if len(dev.sentDirect) != 2 {
		t.Fatalf("sentDirect = %d, want 2", len(dev.sentDirect))
	}
	recipients := map[uuid.UUID]bool{dev.sentDirect[0].recipient: true, dev.sentDirect[1].recipient: true}
	if !recipients[senderA] || !recipients[senderB] {
		t.Errorf("recipients = %v, want %v and %v", recipients, senderA, senderB)
	}
}

// TestMarkRead_NothingUnreadDoesNothing checks that a conversation
// with nothing unread sends no receipts.
func TestMarkRead_NothingUnreadDoesNothing(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.MarkRead(t.Context(), domain.Conversation{RemoteID: "conv-1", Unread: 0}); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if len(dev.sentDirect) != 0 {
		t.Errorf("sentDirect = %d, want 0", len(dev.sentDirect))
	}
}

// TestMarkRead_NotConnected checks that nothing connected reports an
// error rather than panicking.
func TestMarkRead_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.MarkRead(t.Context(), domain.Conversation{Unread: 1}); err == nil {
		t.Error("MarkRead() with nothing connected succeeded, want an error")
	}
}

// TestMarkRead_SkipsUnparseableMessageIDs checks that a saved message
// id that does not parse is skipped rather than failing the whole
// call.
func TestMarkRead_SkipsUnparseableMessageIDs(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: "conv-1", Unread: 1}
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, "not-a-message-id", "guid-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if len(dev.sentDirect) != 0 {
		t.Errorf("sentDirect = %d, want 0", len(dev.sentDirect))
	}
}

// TestMarkRead_DeviceErrorPropagates checks that a failure sending a
// read receipt is returned rather than silently ignored.
func TestMarkRead_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = errors.New("send failed")
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: "conv-1", Unread: 1}
	id := messageRemoteID(sender, 1)
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, id, "guid-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := c.MarkRead(t.Context(), conv); err == nil {
		t.Error("MarkRead() with a failing send succeeded, want an error")
	}
}
