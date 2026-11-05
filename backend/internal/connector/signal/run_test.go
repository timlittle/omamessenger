package signal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestRun_OpenFails checks that Run reports an open failure without
// ever reaching the connect step.
func TestRun_OpenFails(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice(uuid.New()))
	wantErr := errors.New("boom")
	c.open = func(context.Context, func(events.SignalEvent) bool, func(connector.AuthStep)) (device, bool, error) {
		return nil, false, wantErr
	}

	if err := c.Run(t.Context(), &connectortest.Sink{}); err == nil {
		t.Error("Run() succeeded despite open failing")
	}
}

// TestRun_ConnectFails checks that Run reports a failure to start the
// receive loops, and still closes the session it opened.
func TestRun_ConnectFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.startErr = errors.New("no network")
	c := newTestConnector(dev)

	if err := c.Run(t.Context(), &connectortest.Sink{}); err == nil {
		t.Error("Run() succeeded despite startReceiveLoops failing")
	}
	if !dev.closed {
		t.Error("Run() did not close the session after failing to connect")
	}
}

// TestRun_FreshLinkImportsHistory checks that a fresh link runs the
// history import alongside the live connection.
func TestRun_FreshLinkImportsHistory(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}}}
	dev.chats = []*sstore.BackupChat{{Chat: &backuppb.Chat{Id: 10, RecipientId: 1}}}

	c := newTestConnector(dev)
	c.open = testOpener(dev, true)

	ctx, cancel := context.WithCancel(t.Context())
	sink := &connectortest.Sink{}

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, sink) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := sink.ConversationFor(friend.String()); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("history import never reported the imported chat")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	<-done
}

// TestRun_RefusesSecondConcurrentRun checks the same guard the
// conformance suite checks, called directly so it does not depend on
// the conformance suite's own timing.
func TestRun_RefusesSecondConcurrentRun(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice(uuid.New()))
	c.running = true

	if err := c.Run(t.Context(), &connectortest.Sink{}); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("Run() error = %v, want ErrAlreadyRunning", err)
	}
}

// TestReportStatus_Disconnected checks that a disconnected or error
// status reports "connecting" rather than ending the run.
func TestReportStatus_Disconnected(t *testing.T) {
	t.Parallel()

	c := &Connector{account: domain.Account{ID: "acc1"}}
	sink := &connectortest.Sink{}

	status := signalmeow.SignalConnectionStatus{Event: signalmeow.SignalConnectionEventDisconnected}
	terminal, err := c.reportStatus(t.Context(), sink, status)
	if terminal || err != nil {
		t.Errorf("reportStatus(disconnected) = %t, %v, want false, nil", terminal, err)
	}
	if !sink.Has("status acc1 connecting") {
		t.Errorf("status not reported: %v", sink.Lines())
	}
}

// TestReportStatus_FatalError checks that a fatal error status ends
// Run with an error.
func TestReportStatus_FatalError(t *testing.T) {
	t.Parallel()

	c := &Connector{account: domain.Account{ID: "acc1"}}
	status := signalmeow.SignalConnectionStatus{Event: signalmeow.SignalConnectionEventFatalError, Err: errors.New("fatal")}

	terminal, err := c.reportStatus(t.Context(), &connectortest.Sink{}, status)
	if !terminal || err == nil {
		t.Errorf("reportStatus(fatal) = %t, %v, want terminal with an error", terminal, err)
	}
}
