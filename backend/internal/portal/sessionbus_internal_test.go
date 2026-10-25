// dbusBus's own OpenFile and Wait, and the signal-matching helpers they
// use, never run unless something dials a real session bus: every
// black-box test in portal_test.go replaces the whole Bus with a fake
// instead, so it never reaches this production implementation at all.
// This white-box test drives dbusBus directly, over a hand-written fake
// dbus.BusObject and a plain Go channel in place of a live connection's
// signals, instead of reaching a real session bus (see privacy.md and
// concurrency.md: no test may do that).
package portal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeObject is a hand-written stand-in for dbus.BusObject. Embedding a
// nil dbus.BusObject satisfies every method OpenFile never calls;
// call scripts the one method it does.
type fakeObject struct {
	dbus.BusObject
	call func(method string, flags dbus.Flags, args ...any) *dbus.Call
}

// Call records nothing itself; it defers to the fake's own scripted
// call func.
func (f fakeObject) Call(method string, flags dbus.Flags, args ...any) *dbus.Call {
	return f.call(method, flags, args...)
}

func TestDbusBusOpenFile_ReturnsTheRequestHandle(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotArgs []any
	obj := fakeObject{call: func(method string, _ dbus.Flags, args ...any) *dbus.Call {
		gotMethod, gotArgs = method, args
		return &dbus.Call{Body: []any{dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1/t")}}
	}}
	b := &dbusBus{obj: obj}

	handle, err := b.OpenFile(context.Background(), "Attach a file", []Filter{{Name: "Images", Globs: []string{"*.jpg"}}})
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if handle != "/org/freedesktop/portal/desktop/request/1/t" {
		t.Errorf("handle = %q, want the request path the call returned", handle)
	}
	if gotMethod != fileChooserInterface+".OpenFile" {
		t.Errorf("method = %q, want %q", gotMethod, fileChooserInterface+".OpenFile")
	}
	if len(gotArgs) != 3 {
		t.Errorf("args = %v, want the 3 positional arguments OpenFile takes", gotArgs)
	}
}

func TestDbusBusOpenFile_OmitsFiltersWhenThereAreNone(t *testing.T) {
	t.Parallel()

	var gotArgs []any
	obj := fakeObject{call: func(_ string, _ dbus.Flags, args ...any) *dbus.Call {
		gotArgs = args
		return &dbus.Call{Body: []any{dbus.ObjectPath("/req/1")}}
	}}
	b := &dbusBus{obj: obj}

	if _, err := b.OpenFile(context.Background(), "t", nil); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	options, ok := gotArgs[2].(map[string]dbus.Variant)
	if !ok {
		t.Fatalf("options arg = %T, want map[string]dbus.Variant", gotArgs[2])
	}
	if _, has := options["filters"]; has {
		t.Error("options carries a filters entry with no filters given")
	}
}

func TestDbusBusOpenFile_WrapsAFailedCall(t *testing.T) {
	t.Parallel()

	obj := fakeObject{call: func(string, dbus.Flags, ...any) *dbus.Call {
		return &dbus.Call{Err: errors.New("no portal running")}
	}}
	b := &dbusBus{obj: obj}

	if _, err := b.OpenFile(context.Background(), "t", nil); err == nil {
		t.Error("OpenFile over a failing call succeeded")
	}
}

func TestDbusBusWait_ReturnsTheURIOnResponse(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 1)
	sig <- &dbus.Signal{
		Sender: ":1.1", Path: "/req/1", Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///tmp/a.jpg"})}},
	}
	b := &dbusBus{sig: sig, sender: ":1.1"}

	uri, err := b.Wait(context.Background(), "/req/1")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if uri != "file:///tmp/a.jpg" {
		t.Errorf("uri = %q, want file:///tmp/a.jpg", uri)
	}
}

func TestDbusBusWait_ReportsCancelledOnNonZeroCode(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 1)
	sig <- &dbus.Signal{Sender: ":1.1", Path: "/req/1", Name: sigResponse, Body: []any{uint32(1), map[string]dbus.Variant{}}}
	b := &dbusBus{sig: sig, sender: ":1.1"}

	if _, err := b.Wait(context.Background(), "/req/1"); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestDbusBusWait_IgnoresSignalsForAnotherRequest(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{
		Sender: ":1.1", Path: "/req/other", Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///tmp/wrong.jpg"})}},
	}
	sig <- &dbus.Signal{
		Sender: ":1.1", Path: "/req/1", Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///tmp/right.jpg"})}},
	}
	b := &dbusBus{sig: sig, sender: ":1.1"}

	uri, err := b.Wait(context.Background(), "/req/1")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if uri != "file:///tmp/right.jpg" {
		t.Errorf("uri = %q, want the response for the right request", uri)
	}
}

// TestDbusBusWait_IgnoresASignalFromAnotherSender reproduces a spoofed
// Response sent by any other local process on the session bus: once
// sender is known, Wait must never trust a signal from anyone else,
// however well-formed and however correct its own request path is.
func TestDbusBusWait_IgnoresASignalFromAnotherSender(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal, 2)
	sig <- &dbus.Signal{
		Sender: ":1.99", Path: "/req/1", Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///tmp/spoofed.jpg"})}},
	}
	sig <- &dbus.Signal{
		Sender: ":1.1", Path: "/req/1", Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///tmp/real.jpg"})}},
	}
	b := &dbusBus{sig: sig, sender: ":1.1"}

	uri, err := b.Wait(context.Background(), "/req/1")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if uri != "file:///tmp/real.jpg" {
		t.Errorf("uri = %q, want only the trusted sender's own response", uri)
	}
}

func TestDbusBusWait_ReturnsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &dbusBus{sig: make(chan *dbus.Signal)}

	if _, err := b.Wait(ctx, "/req/1"); err == nil {
		t.Error("Wait with an already-cancelled context succeeded")
	}
}

func TestDbusBusWait_ReturnsWhenTheSignalChannelCloses(t *testing.T) {
	t.Parallel()

	sig := make(chan *dbus.Signal)
	close(sig)
	b := &dbusBus{sig: sig}

	if _, err := b.Wait(context.Background(), "/req/1"); err == nil {
		t.Error("Wait over a closed signal channel succeeded")
	}
}

func TestMatchResponse_FalseForAnUnrelatedSignalName(t *testing.T) {
	t.Parallel()

	matched, _, _ := matchResponse(&dbus.Signal{Name: "org.freedesktop.portal.Request.SomethingElse"})
	if matched {
		t.Error("matchResponse matched an unrelated signal name")
	}
}

// TestMatchResponse_FalseWhenTheBodyHasOnlyOneElement reproduces a
// spoofed Response signal that carries only the response code and no
// results at all. Indexing sig.Body[1] without checking its length
// first panics, which would crash the whole helper over one malformed
// signal any local process can send on the session bus.
func TestMatchResponse_FalseWhenTheBodyHasOnlyOneElement(t *testing.T) {
	t.Parallel()

	matched, _, err := matchResponse(&dbus.Signal{Name: sigResponse, Body: []any{uint32(0)}})
	if !matched || err == nil {
		t.Errorf("matchResponse(short body) = matched %t err %v, want matched true, an error", matched, err)
	}
}

func TestMatchResponse_FalseWhenTheCodeIsNotAUint32(t *testing.T) {
	t.Parallel()

	matched, _, err := matchResponse(&dbus.Signal{Name: sigResponse, Body: []any{"not-a-uint32", map[string]dbus.Variant{}}})
	if !matched || err == nil {
		t.Errorf("matchResponse(bad code type) = matched %t err %v, want matched true, an error", matched, err)
	}
}

func TestMatchResponse_FalseWhenResultsIsNotAMap(t *testing.T) {
	t.Parallel()

	matched, _, err := matchResponse(&dbus.Signal{Name: sigResponse, Body: []any{uint32(0), "not-a-map"}})
	if !matched || err == nil {
		t.Errorf("matchResponse(bad results type) = matched %t err %v, want matched true, an error", matched, err)
	}
}

func TestMatchResponse_CancelledWhenUrisIsMissing(t *testing.T) {
	t.Parallel()

	_, _, err := matchResponse(&dbus.Signal{Name: sigResponse, Body: []any{uint32(0), map[string]dbus.Variant{}}})
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestMatchResponse_CancelledWhenUrisIsEmpty(t *testing.T) {
	t.Parallel()

	_, _, err := matchResponse(&dbus.Signal{
		Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{})}},
	})
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestMatchResponse_CancelledWhenUrisIsNotAStringSlice(t *testing.T) {
	t.Parallel()

	_, _, err := matchResponse(&dbus.Signal{
		Name: sigResponse,
		Body: []any{uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant(uint32(1))}},
	})
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestToPortalFilters_ConvertsNamesAndGlobs(t *testing.T) {
	t.Parallel()

	got := toPortalFilters([]Filter{{Name: "Images", Globs: []string{"*.jpg", "*.png"}}})
	if len(got) != 1 || got[0].Name != "Images" || len(got[0].Rules) != 2 {
		t.Fatalf("toPortalFilters = %+v, want one group named Images with 2 rules", got)
	}
	if got[0].Rules[0].Kind != 0 || got[0].Rules[0].Pattern != "*.jpg" {
		t.Errorf("rule 0 = %+v, want kind 0 (glob), pattern *.jpg", got[0].Rules[0])
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

// TestDbusBusOpenFile_ResolvesThePortalsUniqueName claims the portal's
// own bus name on a throwaway dbus-daemon, then confirms OpenFile
// resolves that connection's own unique bus name: the one Wait later
// refuses to accept a signal from any other sender. The claiming
// connection implements no actual OpenFile method, so OpenFile's own
// call to it is expected to fail; only resolving the owner, which
// OpenFile does before attempting that call, is under test here.
func TestDbusBusOpenFile_ResolvesThePortalsUniqueName(t *testing.T) {
	startTestBus(t)

	owner, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("connect to claim the name: %v", err)
	}
	defer func() { _ = owner.Close() }() // the test bus is about to be killed anyway

	if _, err := owner.RequestName(desktopInterface, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}

	bus, release, err := dialSessionBus(context.Background())
	if err != nil {
		t.Fatalf("dialSessionBus: %v", err)
	}
	defer release()

	b, ok := bus.(*dbusBus)
	if !ok {
		t.Fatal("dialSessionBus did not return a *dbusBus")
	}

	_, _ = b.OpenFile(context.Background(), "t", nil)

	if !strings.HasPrefix(b.sender, ":") {
		t.Errorf("sender = %q, want the claiming connection's own unique bus name", b.sender)
	}
}
