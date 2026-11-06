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

// TestDeleteMessages_ForEveryoneSendsDelete checks that deleting self's
// own recent message for everyone sends Signal's own Delete field.
func TestDeleteMessages_ForEveryoneSendsDelete(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	other := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(self, uint64(time.Now().UnixMilli()))
	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: other.String()}, []string{remoteID}, true)
	if err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}

	if len(dev.sentDirect) != 1 || dev.sentDirect[0].content.GetDataMessage().GetDelete() == nil {
		t.Errorf("sentDirect = %+v, want one Delete message", dev.sentDirect)
	}
}

// TestDeleteMessages_ForMeDoesNothing checks that "delete for me"
// sends nothing to Signal, since it has no protocol message for it.
func TestDeleteMessages_ForMeDoesNothing(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, []string{"anything"}, false)
	if err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}
	if len(dev.sentDirect) != 0 {
		t.Errorf("sentDirect = %+v, want none", dev.sentDirect)
	}
}

// TestDeleteMessages_RefusesSomeoneElsesMessage checks that deleting
// for everyone a message sent by another account is refused.
func TestDeleteMessages_RefusesSomeoneElsesMessage(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: sender.String()}, []string{remoteID}, true)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages() error = %v, want connector.ErrDeleteUnsupported", err)
	}
}

// TestDeleteMessages_RefusesExpired checks that deleting for everyone
// a message past Signal's own delete window is refused.
func TestDeleteMessages_RefusesExpired(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	old := time.Now().Add(-signalDeleteWindow - time.Hour)
	remoteID := messageRemoteID(self, uint64(old.UnixMilli()))
	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: self.String()}, []string{remoteID}, true)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages() error = %v, want connector.ErrDeleteUnsupported", err)
	}
}

// TestDeleteMessages_AllowsADayOldMessage checks that a message sent
// 12 hours ago can still be deleted for everyone, as Signal allows a day.
func TestDeleteMessages_AllowsADayOldMessage(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	sent := time.Now().Add(-12 * time.Hour)
	remoteID := messageRemoteID(self, uint64(sent.UnixMilli()))
	if err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: self.String()}, []string{remoteID}, true); err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}
	if len(dev.sentDirect) != 1 {
		t.Errorf("sentDirect = %d messages, want 1", len(dev.sentDirect))
	}
}

// TestDeleteMessages_NotConnected checks that nothing connected
// reports an error rather than panicking, even for "delete for me"
// which would otherwise return nil before checking the session.
func TestDeleteMessages_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.DeleteMessages(t.Context(), domain.Conversation{}, []string{"m1"}, true); err == nil {
		t.Error("DeleteMessages(forEveryone) with nothing connected succeeded, want an error")
	}
}

// TestDeleteMessages_RejectsMalformedMessageID checks that a message
// id that does not parse is refused with an error.
func TestDeleteMessages_RejectsMalformedMessageID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, []string{"not-a-message-id"}, true)
	if err == nil {
		t.Error("DeleteMessages() with a malformed message id succeeded, want an error")
	}
}

// TestDeleteMessages_DeviceErrorPropagates checks that a failure
// sending the delete is returned rather than silently ignored.
func TestDeleteMessages_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	dev.sendDirectErr = errors.New("send failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	remoteID := messageRemoteID(self, uint64(time.Now().UnixMilli()))
	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: self.String()}, []string{remoteID}, true)
	if err == nil {
		t.Error("DeleteMessages() with a failing send succeeded, want an error")
	}
}
