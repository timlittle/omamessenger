// dbusOpenBus's own OpenURI, OpenFile and Wait, and the signal-matching
// helpers they use, never run unless something dials a real session
// bus: every black-box test in openuri_test.go replaces the whole
// OpenBus with a fake instead, so it never reaches this production
// implementation at all. This white-box test drives dbusOpenBus
// directly, over a hand-written fake dbus.BusObject and a plain Go
// channel in place of a live connection's signals, instead of reaching
// a real session bus (see privacy.md and concurrency.md: no test may
// do that).
package portal

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeOpenObject is a hand-written stand-in for dbus.BusObject.
// Embedding a nil dbus.BusObject satisfies every method OpenURI and
// OpenFile never call; call scripts the one method they do.
type fakeOpenObject struct {
	dbus.BusObject
	call func(method string, flags dbus.Flags, args ...any) *dbus.Call
}

// Call records nothing itself; it defers to the fake's own scripted
// call func.
func (f fakeOpenObject) Call(method string, flags dbus.Flags, args ...any) *dbus.Call {
	return f.call(method, flags, args...)
}

func TestDbusOpenBusOpenURI_ReturnsTheRequestHandle(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotArgs []any
	obj := fakeOpenObject{call: func(method string, _ dbus.Flags, args ...any) *dbus.Call {
		gotMethod, gotArgs = method, args
		return &dbus.Call{Body: []any{dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1/t")}}
	}}
	b := &dbusOpenBus{obj: obj}

	handle, err := b.OpenURI(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("OpenURI: %v", err)
	}
	if handle != "/org/freedesktop/portal/desktop/request/1/t" {
		t.Errorf("handle = %q, want the request path the call returned", handle)
	}
	if gotMethod != openURIInterface+".OpenURI" {
		t.Errorf("method = %q, want %q", gotMethod, openURIInterface+".OpenURI")
	}
	if len(gotArgs) != 3 || gotArgs[1] != "https://example.com" {
		t.Errorf("args = %v, want the 3 positional arguments OpenURI takes, with the uri second", gotArgs)
	}
}

func TestDbusOpenBusOpenURI_WrapsAFailedCall(t *testing.T) {
	t.Parallel()

	obj := fakeOpenObject{call: func(string, dbus.Flags, ...any) *dbus.Call {
		return &dbus.Call{Err: errors.New("no portal running")}
	}}
	b := &dbusOpenBus{obj: obj}

	if _, err := b.OpenURI(context.Background(), "https://example.com"); err == nil {
		t.Error("OpenURI over a failing call succeeded")
	}
}

func TestDbusOpenBusOpenFile_PassesTheFilesOwnDescriptor(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "photo")
	if err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var gotMethod string
	var gotArgs []any
	obj := fakeOpenObject{call: func(method string, _ dbus.Flags, args ...any) *dbus.Call {
		gotMethod, gotArgs = method, args
		return &dbus.Call{Body: []any{dbus.ObjectPath("/req/1")}}
	}}
	b := &dbusOpenBus{obj: obj}

	if _, err := b.OpenFile(context.Background(), f); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if gotMethod != openURIInterface+".OpenFile" {
		t.Errorf("method = %q, want %q", gotMethod, openURIInterface+".OpenFile")
	}
	if len(gotArgs) != 3 {
		t.Fatalf("args = %v, want the 3 positional arguments OpenFile takes", gotArgs)
	}
	if fd, ok := gotArgs[1].(dbus.UnixFD); !ok || fd != dbus.UnixFD(f.Fd()) {
		t.Errorf("args[1] = %v, want the file's own descriptor as a dbus.UnixFD", gotArgs[1])
	}
}

func TestDbusOpenBusOpenFile_WrapsAFailedCall(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "photo")
	if err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	defer func() { _ = f.Close() }()

	obj := fakeOpenObject{call: func(string, dbus.Flags, ...any) *dbus.Call {
		return &dbus.Call{Err: errors.New("no portal running")}
	}}
	b := &dbusOpenBus{obj: obj}

	if _, err := b.OpenFile(context.Background(), f); err == nil {
		t.Error("OpenFile over a failing call succeeded")
	}
}

func TestDbusOpenBusWait_ReturnsNilOnSuccess(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 1)
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/1", Name: sigResponse, Body: []any{uint32(0)}}
	b := &dbusOpenBus{sig: sig, sender: ":1.1"}

	if err := b.Wait(context.Background(), "/req/1"); err != nil {
		t.Errorf("Wait: %v, want nil for a successful response", err)
	}
}

func TestDbusOpenBusWait_ReturnsAnErrorOnNonZeroCode(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 1)
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/1", Name: sigResponse, Body: []any{uint32(1)}}
	b := &dbusOpenBus{sig: sig, sender: ":1.1"}

	if err := b.Wait(context.Background(), "/req/1"); err == nil {
		t.Error("Wait over a non-zero response code succeeded")
	}
}

func TestDbusOpenBusWait_IgnoresSignalsForAnotherRequest(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/other", Name: sigResponse, Body: []any{uint32(1)}}
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/1", Name: sigResponse, Body: []any{uint32(0)}}
	b := &dbusOpenBus{sig: sig, sender: ":1.1"}

	if err := b.Wait(context.Background(), "/req/1"); err != nil {
		t.Errorf("Wait: %v, want nil - only the matching request's own response should be used", err)
	}
}

// TestDbusOpenBusWait_IgnoresASignalFromAnotherSender reproduces a
// spoofed Response sent by any other local process on the session
// bus: once sender is known, Wait must never trust a signal from
// anyone else, however correct its own request path is.
func TestDbusOpenBusWait_IgnoresASignalFromAnotherSender(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{Sender: ":1.99", Path: "/req/1", Name: sigResponse, Body: []any{uint32(0)}}
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/1", Name: sigResponse, Body: []any{uint32(1)}}
	b := &dbusOpenBus{sig: sig, sender: ":1.1"}

	if err := b.Wait(context.Background(), "/req/1"); err == nil {
		t.Error("Wait trusted a spoofed response from an unexpected sender over the real sender's own failure")
	}
}

func TestDbusOpenBusWait_ReturnsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &dbusOpenBus{sig: make(chan *dbus.Signal)}

	if err := b.Wait(ctx, "/req/1"); err == nil {
		t.Error("Wait with an already-cancelled context succeeded")
	}
}

func TestDbusOpenBusWait_ReturnsWhenTheSignalChannelCloses(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal)
	close(sig)
	b := &dbusOpenBus{sig: sig}

	if err := b.Wait(context.Background(), "/req/1"); err == nil {
		t.Error("Wait over a closed signal channel succeeded")
	}
}

func TestMatchOpenResponse_FalseForAnUnrelatedSignalName(t *testing.T) {
	t.Parallel()

	matched, _ := matchOpenResponse(&dbus.Signal{Name: "org.freedesktop.portal.Request.SomethingElse"})
	if matched {
		t.Error("matchOpenResponse matched an unrelated signal name")
	}
}

// TestMatchOpenResponse_FalseWhenTheBodyIsEmpty reproduces a spoofed
// Response signal that carries no response code at all. Indexing
// sig.Body[0] without checking its length first panics, which would
// crash the whole helper over one malformed signal any local process
// can send on the session bus.
func TestMatchOpenResponse_FalseWhenTheBodyIsEmpty(t *testing.T) {
	t.Parallel()

	matched, err := matchOpenResponse(&dbus.Signal{Name: sigResponse, Body: []any{}})
	if !matched || err == nil {
		t.Errorf("matchOpenResponse(empty body) = matched %t err %v, want matched true, an error", matched, err)
	}
}

func TestMatchOpenResponse_FalseWhenTheCodeIsNotAUint32(t *testing.T) {
	t.Parallel()

	matched, err := matchOpenResponse(&dbus.Signal{Name: sigResponse, Body: []any{"not-a-uint32"}})
	if !matched || err == nil {
		t.Errorf("matchOpenResponse(bad code type) = matched %t err %v, want matched true, an error", matched, err)
	}
}

// TestDialOpenBus_ConnectsAndWatchesSignalsOverARealBus drives
// dialOpenBus against a throwaway dbus-daemon (see helpers_test.go),
// rather than the developer's own session bus, to cover the connect,
// AddMatchSignal and release path that only ever runs against a live
// connection.
func TestDialOpenBus_ConnectsAndWatchesSignalsOverARealBus(t *testing.T) {
	startTestBus(t)

	b, release, err := dialOpenBus(context.Background())
	if err != nil {
		t.Fatalf("dialOpenBus: %v", err)
	}
	defer release()

	if b == nil {
		t.Fatal("dialOpenBus returned a nil OpenBus alongside a nil error")
	}
}

// TestDbusOpenBusOpenURI_ResolvesThePortalsUniqueName claims the
// portal's own bus name on a throwaway dbus-daemon, then confirms
// OpenURI resolves that connection's own unique bus name: the one
// Wait later refuses to accept a signal from any other sender. The
// claiming connection implements no actual OpenURI method, so
// OpenURI's own call to it is expected to fail; only resolving the
// owner, which OpenURI does before attempting that call, is under
// test here.
func TestDbusOpenBusOpenURI_ResolvesThePortalsUniqueName(t *testing.T) {
	startTestBus(t)

	owner, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("connect to claim the name: %v", err)
	}
	defer func() { _ = owner.Close() }() // the test bus is about to be killed anyway

	if _, err := owner.RequestName(desktopInterface, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}

	bus, release, err := dialOpenBus(context.Background())
	if err != nil {
		t.Fatalf("dialOpenBus: %v", err)
	}
	defer release()

	b, ok := bus.(*dbusOpenBus)
	if !ok {
		t.Fatal("dialOpenBus did not return a *dbusOpenBus")
	}

	_, _ = b.OpenURI(context.Background(), "https://example.com")

	if !strings.HasPrefix(b.sender, ":") {
		t.Errorf("sender = %q, want the claiming connection's own unique bus name", b.sender)
	}
}
