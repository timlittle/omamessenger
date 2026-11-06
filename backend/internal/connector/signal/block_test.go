package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSetBlocked_SendsBlockSync checks that blocking sends a
// message-request-response sync message naming the contact's own
// account, to self's own account.
func TestSetBlocked_SendsBlockSync(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	other := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.SetBlocked(t.Context(), domain.Conversation{RemoteID: other.String()}, true); err != nil {
		t.Fatalf("SetBlocked(true) error = %v", err)
	}

	if len(dev.sentDirect) != 1 || dev.sentDirect[0].recipient != self {
		t.Fatalf("sentDirect = %+v, want one sync message to self", dev.sentDirect)
	}

	resp := dev.sentDirect[0].content.GetSyncMessage().GetMessageRequestResponse()
	if resp.GetType() != signalpb.SyncMessage_MessageRequestResponse_BLOCK {
		t.Errorf("response type = %v, want BLOCK", resp.GetType())
	}
	if uuid.UUID(resp.GetThreadAciBinary()) != other {
		t.Errorf("response thread aci = %v, want %v", resp.GetThreadAciBinary(), other)
	}
}

// TestSetBlocked_UnblockSendsAccept checks that unblocking sends
// ACCEPT rather than BLOCK.
func TestSetBlocked_UnblockSendsAccept(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.SetBlocked(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, false); err != nil {
		t.Fatalf("SetBlocked(false) error = %v", err)
	}

	resp := dev.sentDirect[0].content.GetSyncMessage().GetMessageRequestResponse()
	if resp.GetType() != signalpb.SyncMessage_MessageRequestResponse_ACCEPT {
		t.Errorf("response type = %v, want ACCEPT", resp.GetType())
	}
}

// TestSetBlocked_NotConnected checks that nothing connected reports
// an error rather than panicking.
func TestSetBlocked_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.SetBlocked(t.Context(), domain.Conversation{}, true); err == nil {
		t.Error("SetBlocked() with nothing connected succeeded, want an error")
	}
}

// TestSetBlocked_InvalidConversationID checks that a conversation
// remote id that is not a plain account id is refused with an error.
func TestSetBlocked_InvalidConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.SetBlocked(t.Context(), domain.Conversation{RemoteID: "not-an-account-id"}, true); err == nil {
		t.Error("SetBlocked() with an invalid id succeeded, want an error")
	}
}

// TestSetBlocked_DeviceErrorPropagates checks that a failure sending
// the sync message is returned rather than silently ignored.
func TestSetBlocked_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = errors.New("sync failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.SetBlocked(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, true); err == nil {
		t.Error("SetBlocked() with a failing sync succeeded, want an error")
	}
}
