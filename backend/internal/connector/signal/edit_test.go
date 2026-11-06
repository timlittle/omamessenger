package signal

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestEditMessage_SendsEdit checks that editing self's own recent
// message sends Signal's edit protocol message to the right chat.
func TestEditMessage_SendsEdit(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	other := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(self, uint64(time.Now().UnixMilli()))
	if err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: other.String()}, remoteID, "new text", nil); err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}

	if len(dev.sentDirect) != 1 {
		t.Fatalf("sentDirect = %d, want 1", len(dev.sentDirect))
	}
	edit := dev.sentDirect[0].content.GetEditMessage()
	if edit.GetDataMessage().GetBody() != "new text" {
		t.Errorf("edited body = %q, want %q", edit.GetDataMessage().GetBody(), "new text")
	}
}

// TestEditMessage_RefusesSomeoneElsesMessage checks that editing a
// message sent by another account is refused.
func TestEditMessage_RefusesSomeoneElsesMessage(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: sender.String()}, remoteID, "new text", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage() error = %v, want connector.ErrEditUnsupported", err)
	}
}

// TestEditMessage_RefusesExpired checks that editing a message past
// Signal's own edit window is refused.
func TestEditMessage_RefusesExpired(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	old := time.Now().Add(-signalEditWindow - time.Hour)
	remoteID := messageRemoteID(self, uint64(old.UnixMilli()))
	err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: self.String()}, remoteID, "new text", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage() error = %v, want connector.ErrEditUnsupported", err)
	}
}

// TestEditMessage_NotConnected checks that nothing connected reports
// an error rather than panicking.
func TestEditMessage_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.EditMessage(t.Context(), domain.Conversation{}, "m1", "new text", nil); err == nil {
		t.Error("EditMessage() with nothing connected succeeded, want an error")
	}
}

// TestEditMessage_RejectsMalformedMessageID checks that a message id
// that does not parse is refused with an error.
func TestEditMessage_RejectsMalformedMessageID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, "not-a-message-id", "new text", nil)
	if err == nil {
		t.Error("EditMessage() with a malformed message id succeeded, want an error")
	}
}

// TestEditMessage_DeviceErrorPropagates checks that a failure
// sending the edit is returned rather than silently ignored.
func TestEditMessage_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	dev.sendDirectErr = errors.New("send failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(self, uint64(time.Now().UnixMilli()))
	err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: self.String()}, remoteID, "new text", nil)
	if err == nil {
		t.Error("EditMessage() with a failing send succeeded, want an error")
	}
}
