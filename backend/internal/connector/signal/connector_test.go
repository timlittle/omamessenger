package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSend_NotImplemented checks that sending reports it is not
// implemented yet, rather than silently doing nothing: S4 adds
// sending.
func TestSend_NotImplemented(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice(uuid.New()))
	err := c.Send(t.Context(), domain.Conversation{}, domain.Message{})
	if !errors.Is(err, errNotImplemented) {
		t.Errorf("Send() error = %v, want errNotImplemented", err)
	}
}

// TestMarkRead_NotImplemented checks the same for marking a
// conversation read.
func TestMarkRead_NotImplemented(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice(uuid.New()))
	err := c.MarkRead(t.Context(), domain.Conversation{})
	if !errors.Is(err, errNotImplemented) {
		t.Errorf("MarkRead() error = %v, want errNotImplemented", err)
	}
}

// TestLogout_Connected checks that Logout unlinks the connected
// device.
func TestLogout_Connected(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	if err := c.Logout(t.Context()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if !dev.unlinked {
		t.Error("Logout() did not unlink the device")
	}
}

// TestLogout_NotConnected checks that Logout with nothing connected
// reports success, since there is nothing to unlink.
func TestLogout_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}

	if err := c.Logout(t.Context()); err != nil {
		t.Errorf("Logout() error = %v, want nil when nothing is connected", err)
	}
}

// TestStartRun_RefusesSecondConcurrentRun checks that a second
// concurrent Run is refused, matching the conformance suite's own
// check but at the unit level so startRun and endRun are covered
// without going through the whole Run lifecycle twice.
func TestStartRun_RefusesSecondConcurrentRun(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.startRun(); err != nil {
		t.Fatalf("first startRun() error = %v", err)
	}
	defer c.endRun()

	if err := c.startRun(); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second startRun() error = %v, want ErrAlreadyRunning", err)
	}
}

// TestReportStatus_LoggedOutEndsRun checks that a logged-out event
// ends Run with an error, since signalmeow will not recover from this
// on its own.
func TestReportStatus_LoggedOutEndsRun(t *testing.T) {
	t.Parallel()

	c := &Connector{account: domain.Account{ID: "acc1"}}
	status := signalmeow.SignalConnectionStatus{Event: signalmeow.SignalConnectionEventLoggedOut, Err: errors.New("logged out")}
	terminal, err := c.reportStatus(t.Context(), &connectortest.Sink{}, status)
	if !terminal || err == nil {
		t.Errorf("reportStatus(loggedOut) = %t, %v, want terminal with an error", terminal, err)
	}
}
