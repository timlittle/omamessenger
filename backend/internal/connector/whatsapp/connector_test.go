package whatsapp

// Connector's pairing and status handling is driven through a fake
// device rather than Run's exported API alone, because only the
// unexported answers channel and the fake let a test push a QR code,
// switch to a phone number, or simulate WhatsApp's own status events.
// Each test runs inside synctest, as connectortest.CheckLifecycle does,
// so it can wait for Run's goroutine to react without polling or
// sleeping for a fixed amount of real time.

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// newTestConnector returns a connector over dev, ready for Run.
func newTestConnector(dev *fakeDevice) *Connector {
	return &Connector{
		account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp},
		answers: make(chan answer, 1),
		open:    func(context.Context) (device, error) { return dev, nil },
	}
}

func TestRun_PairsByQRThenConnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- newTestConnector(dev).Run(ctx, &sink) }()
		synctest.Wait()

		dev.codes <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "1@abc"}
		synctest.Wait()

		steps := sink.AuthSteps()
		if len(steps) != 1 || steps[0].Kind != "qr" || steps[0].QR == "" {
			t.Fatalf("steps = %+v, want one qr step with a rendered code", steps)
		}

		dev.codes <- whatsmeow.QRChannelItem{Event: "success"}
		synctest.Wait()
		dev.status(statusConnected) // whatsmeow fires this once pairing's handshake finishes
		synctest.Wait()

		if dev.connects != 1 || !sink.Has("status wa-1 "+domain.AccountConnected) {
			t.Errorf("connects = %d, events = %q", dev.connects, sink.Lines())
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestRun_SwitchesToPhoneAndReportsTheLinkCode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.pairCode = "ABCD-1234"
		var sink connectortest.Sink

		c := newTestConnector(dev)
		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- c.Run(ctx, &sink) }()
		synctest.Wait()

		if err := c.SubmitAuth(t.Context(), "phone", "+15551234567"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		steps := sink.AuthSteps()
		last := steps[len(steps)-1]
		if last.Kind != "linkcode" || last.Hint == "" {
			t.Fatalf("last step = %+v, want a linkcode step with a hint", last)
		}
		if got := dev.pairedPhones; len(got) != 1 || got[0] != "+15551234567" {
			t.Errorf("paired phones = %v, want [+15551234567]", got)
		}

		dev.codes <- whatsmeow.QRChannelItem{Event: "success"}
		synctest.Wait()
		dev.status(statusConnected)
		synctest.Wait()
		if !sink.Has("status wa-1 " + domain.AccountConnected) {
			t.Errorf("events = %q, want a connected status", sink.Lines())
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestRun_RetriesAfterABadPhoneNumber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.pairErr = errors.New("phone number is too short")
		var sink connectortest.Sink

		c := newTestConnector(dev)
		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- c.Run(ctx, &sink) }()
		synctest.Wait()

		if err := c.SubmitAuth(t.Context(), "phone", "123"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		first := sink.AuthSteps()[0]
		if first.Kind != "phone" {
			t.Fatalf("step after a bad number = %+v, want another phone step", first)
		}

		dev.pairErr = nil
		dev.pairCode = "WXYZ-9876"
		if err := c.SubmitAuth(t.Context(), "phone", "+15551234567"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		steps := sink.AuthSteps()
		if last := steps[len(steps)-1]; last.Kind != "linkcode" {
			t.Fatalf("last step = %+v, want linkcode after retrying", last)
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestRun_AlreadyPairedConnectsWithoutAsking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- newTestConnector(dev).Run(ctx, &sink) }()
		synctest.Wait()

		if !sink.Has("status wa-1 " + domain.AccountConnected) {
			t.Errorf("events = %q, want a connected status", sink.Lines())
		}
		if len(sink.AuthSteps()) != 0 {
			t.Errorf("auth steps = %v, want none for an already-paired account", sink.AuthSteps())
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestRun_EndsWhenWhatsAppWillNotReconnectOnItsOwn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { done <- newTestConnector(dev).Run(ctx, &sink) }()
		synctest.Wait()

		dev.status(statusStopped)
		synctest.Wait()

		select {
		case err := <-done:
			if err == nil {
				t.Error("Run returned nil after a permanent disconnect, want an error")
			}
		default:
			t.Fatal("Run did not return after a permanent disconnect")
		}
	})
}

func TestRun_RefusesASecondConcurrentRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		c := newTestConnector(dev)
		var sink connectortest.Sink

		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { done <- c.Run(ctx, &sink) }()
		synctest.Wait()

		if err := c.Run(t.Context(), &connectortest.Sink{}); !errors.Is(err, ErrAlreadyRunning) {
			t.Errorf("second Run = %v, want ErrAlreadyRunning", err)
		}

		cancel()
		synctest.Wait()
		drain(t, done)
	})
}

func TestSubmitAuth_RejectsAnAnswerWhenNotWaiting(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice())
	if err := c.SubmitAuth(t.Context(), "phone", "+1"); !errors.Is(err, errNotPairing) {
		t.Errorf("SubmitAuth = %v, want errNotPairing", err)
	}
}

func TestSend_FailsBeforeConnecting(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice())
	if err := c.Send(t.Context(), domain.Conversation{}, domain.Message{}); !errors.Is(err, errNotConnected) {
		t.Errorf("Send = %v, want errNotConnected", err)
	}
	if err := c.MarkRead(t.Context(), domain.Conversation{}); !errors.Is(err, errNotConnected) {
		t.Errorf("MarkRead = %v, want errNotConnected", err)
	}
}

// drain reads Run's result, failing the test if it is neither nil nor a
// context error, as connectortest.CheckLifecycle does after a cancel.
func drain(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v after cancel, want nil or a context error", err)
		}
	default:
		t.Error("Run did not return promptly after its context was cancelled")
	}
}
