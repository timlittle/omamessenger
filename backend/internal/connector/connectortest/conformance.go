package connectortest

// The checks below are the one suite every connector runs against, so a
// bug in connect, cancel, send progress, incoming fields, duplicate
// deliveries or goroutine cleanup is caught the same way for every
// service. Each check is independent: a connector that cannot run one
// offline can still run the rest.

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// connectWait is slept, inside the synctest bubble, before checking that a
// connector reached "connected". The sleep costs no real time there; it
// only has to outlast any connector's own connect delay.
const connectWait = time.Hour

// CheckLifecycle verifies that Run connects, honours cancellation, refuses
// a second concurrent run and leaves no goroutine behind. newConnector
// must return a fresh connector that has not been run yet; it is called
// once, inside a synctest bubble, so the connector's own delays advance
// instantly.
func CheckLifecycle(t *testing.T, newConnector func(t *testing.T) connector.Connector) {
	t.Helper()

	synctest.Test(t, func(t *testing.T) {
		c := newConnector(t)
		id := c.Account().ID
		sink := &Sink{}
		ctx, cancel := context.WithCancel(t.Context())

		done := make(chan error, 1)
		go func() { done <- c.Run(ctx, sink) }()

		synctest.Wait()
		if !sink.Has("status " + id + " " + domain.AccountConnecting) {
			t.Errorf("Run did not report connecting: %v", sink.Lines())
		}

		time.Sleep(connectWait)
		synctest.Wait()
		if !sink.Has("status " + id + " " + domain.AccountConnected) {
			t.Errorf("Run did not report connected: %v", sink.Lines())
		}

		if err := c.Run(ctx, &Sink{}); err == nil {
			t.Error("a second concurrent Run succeeded, want it refused")
		}

		cancel()
		synctest.Wait()

		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Run returned %v after cancel, want nil or a context error", err)
			}
		default:
			t.Error("Run did not return promptly after its context was cancelled")
		}
	})
}

// CheckSendProgress asserts that the delivery updates sink recorded for a
// message move forward (pending, sent, delivered, read, with failed as a
// terminal state a retry can leave) and that the service's id is known by
// the time the message is reported sent.
func CheckSendProgress(t *testing.T, sink *Sink, localMessageID string) {
	t.Helper()

	updates := sink.Outgoing(localMessageID)
	if len(updates) == 0 {
		t.Fatalf("no delivery updates recorded for message %q", localMessageID)
	}

	from := ""
	for _, u := range updates {
		if from != "" && !domain.StatusAdvances(from, u.Status) {
			t.Errorf("status for %q moved from %q to %q, which is not forward progress", localMessageID, from, u.Status)
		}

		if u.Status == domain.StatusSent && u.RemoteID == "" {
			t.Errorf("message %q was reported sent with no service id", localMessageID)
		}

		from = u.Status
	}
}

// CheckIncoming asserts that an incoming message has the fields the store
// and UI depend on: who sent it, what it says, when, and that it is
// marked as a received message rather than one of our own.
func CheckIncoming(t *testing.T, m domain.Message) {
	t.Helper()

	if m.RemoteID == "" {
		t.Error("incoming message has no remote id")
	}
	if m.SenderID == "" {
		t.Error("incoming message has no sender id")
	}
	if m.SenderName == "" {
		t.Error("incoming message has no sender name")
	}
	if m.Text == "" {
		t.Error("incoming message has no text")
	}
	if m.Created <= 0 {
		t.Errorf("incoming message created = %d, want a positive timestamp", m.Created)
	}
	if m.Outgoing {
		t.Error("incoming message is marked outgoing")
	}
	if m.Status != domain.StatusReceived {
		t.Errorf("incoming message status = %q, want %q", m.Status, domain.StatusReceived)
	}
}

// CheckDuplicates asserts that the same service message, reported twice as
// a real connector does after a reconnect or an update replay, carries the
// same remote id both times, so the store can recognise and drop the copy.
func CheckDuplicates(t *testing.T, first, second domain.Message) {
	t.Helper()

	if first.RemoteID == "" {
		t.Fatal("first message has no remote id to compare")
	}

	if first.RemoteID != second.RemoteID {
		t.Errorf("remote id changed on redelivery: %q then %q", first.RemoteID, second.RemoteID)
	}
}
