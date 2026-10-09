// dbusBus's own Show and Wait, and the signal-matching helpers they use,
// never run unless something dials a real session bus: every black-box
// test in notify_test.go replaces the whole Bus with a fake instead, so
// it never reaches this production implementation at all. This white-box
// test drives dbusBus directly, over a hand-written fake dbus.BusObject
// and a plain Go channel in place of a live connection's signals,
// instead of reaching a real session bus (see privacy.md and
// concurrency.md: no test may do that).
package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeObject is a hand-written stand-in for dbus.BusObject. Embedding a
// nil dbus.BusObject satisfies every method Show never calls; call
// scripts the one method it does.
type fakeObject struct {
	dbus.BusObject
	call func(method string, flags dbus.Flags, args ...any) *dbus.Call
}

// Call records nothing itself; it defers to the fake's own scripted
// call func.
func (f fakeObject) Call(method string, flags dbus.Flags, args ...any) *dbus.Call {
	return f.call(method, flags, args...)
}

func TestDbusBusShow_ReturnsTheIDTheServiceAssigned(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotArgs []any
	obj := fakeObject{call: func(method string, _ dbus.Flags, args ...any) *dbus.Call {
		gotMethod, gotArgs = method, args
		return &dbus.Call{Body: []any{uint32(7)}}
	}}
	b := &dbusBus{obj: obj}

	id, err := b.Show("OmaMessenger", "title", "body", []string{"default", "Open"},
		map[string]dbus.Variant{"category": dbus.MakeVariant("im.received")})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if id != 7 {
		t.Errorf("id = %d, want 7", id)
	}
	if gotMethod != notifyInterface+".Notify" {
		t.Errorf("method = %q, want %q", gotMethod, notifyInterface+".Notify")
	}
	if len(gotArgs) != 8 {
		t.Errorf("args = %v, want the 8 positional arguments Notify takes", gotArgs)
	}
}

func TestDbusBusShow_WrapsAFailedCall(t *testing.T) {
	t.Parallel()

	obj := fakeObject{call: func(string, dbus.Flags, ...any) *dbus.Call {
		return &dbus.Call{Err: errors.New("service refused the call")}
	}}
	b := &dbusBus{obj: obj}

	if _, err := b.Show("a", "t", "b", nil, nil); err == nil {
		t.Error("Show over a failing call succeeded")
	}
}

func TestDbusBusWait_ReturnsTheActionInvokedBeforeClosing(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{Name: sigActionInvoked, Body: []any{uint32(5), "default"}}
	sig <- &dbus.Signal{Name: sigNotificationClosed, Body: []any{uint32(5), uint32(2)}}
	b := &dbusBus{sig: sig}

	action, err := b.Wait(context.Background(), 5)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if action != "default" {
		t.Errorf("action = %q, want %q", action, "default")
	}
}

func TestDbusBusWait_IgnoresSignalsForAnotherNotification(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{Name: sigActionInvoked, Body: []any{uint32(99), "default"}}
	sig <- &dbus.Signal{Name: sigNotificationClosed, Body: []any{uint32(5), uint32(2)}}
	b := &dbusBus{sig: sig}

	action, err := b.Wait(context.Background(), 5)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if action != "" {
		t.Errorf("action = %q, want none: the invoked signal was for a different notification", action)
	}
}

func TestDbusBusWait_IgnoresAnUnrelatedSignal(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{Name: "org.freedesktop.Notifications.Something", Body: []any{uint32(5)}}
	sig <- &dbus.Signal{Name: sigNotificationClosed, Body: []any{uint32(5), uint32(2)}}
	b := &dbusBus{sig: sig}

	if _, err := b.Wait(context.Background(), 5); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestDbusBusWait_ReturnsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &dbusBus{sig: make(chan *dbus.Signal)}

	if _, err := b.Wait(ctx, 1); err == nil {
		t.Error("Wait with an already-cancelled context succeeded")
	}
}

func TestSignalIsFor_FalseWhenTheBodyIsEmpty(t *testing.T) {
	t.Parallel()

	if signalIsFor(&dbus.Signal{Name: sigNotificationClosed}, 1) {
		t.Error("signalIsFor with no body arguments reported a match")
	}
}

func TestSignalIsFor_FalseWhenTheFirstBodyArgumentIsNotAUint32(t *testing.T) {
	t.Parallel()

	sig := &dbus.Signal{Name: sigNotificationClosed, Body: []any{"not-a-uint32"}}
	if signalIsFor(sig, 1) {
		t.Error("signalIsFor with a non-uint32 first argument reported a match")
	}
}

func TestDbusBusWait_ReturnsWhenTheSignalChannelCloses(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal)
	close(sig)
	b := &dbusBus{sig: sig}

	if _, err := b.Wait(context.Background(), 1); err == nil {
		t.Error("Wait over a closed signal channel succeeded")
	}
}

// TestDialSessionBus_ConnectsAndWatchesSignalsOverARealBus drives
// dialSessionBus against a throwaway dbus-daemon (see helpers_test.go),
// rather than the developer's own session bus, to cover the connect,
// AddMatchSignal and release path that only ever runs against a live
// connection.
func TestDialSessionBus_ConnectsAndWatchesSignalsOverARealBus(t *testing.T) {
	startTestBus(t)

	b, release, err := dialSessionBus(context.Background())
	if err != nil {
		t.Fatalf("dialSessionBus: %v", err)
	}
	defer release()

	if b == nil {
		t.Fatal("dialSessionBus returned a nil Bus alongside a nil error")
	}
}

// TestServiceReachable_ReturnsTrueWhenSomethingOwnsTheName claims the
// notification service's own bus name on a throwaway dbus-daemon, then
// confirms ServiceReachable reports it, covering the branch that only
// runs once a real NameHasOwner reply comes back true.
func TestServiceReachable_ReturnsTrueWhenSomethingOwnsTheName(t *testing.T) {
	startTestBus(t)

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("connect to claim the name: %v", err)
	}
	defer func() { _ = conn.Close() }() // the test bus is about to be killed anyway

	if _, err := conn.RequestName(notifyInterface, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}

	if !ServiceReachable(context.Background()) {
		t.Error("ServiceReachable = false with the name owned, want true")
	}
}
