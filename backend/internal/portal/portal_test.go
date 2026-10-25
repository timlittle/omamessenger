// Picker's Dial field replaces a real session bus connection, so every
// test here runs a hand-written fake Bus instead of reaching one, and
// never opens a real file-choosing dialog.
package portal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/timlittle/omamessenger/backend/internal/portal"
)

// fakeBus is a hand-written stand-in for portal.Bus. It records
// OpenFile's arguments and lets a test script Wait's own reply,
// without ever reaching a real session bus.
type fakeBus struct {
	gotTitle   string
	gotFilters []portal.Filter

	openErr error
	handle  dbus.ObjectPath

	uri     string
	waitErr error
}

// OpenFile records its arguments and returns the fake's scripted
// handle and error.
func (b *fakeBus) OpenFile(_ context.Context, title string, filters []portal.Filter) (dbus.ObjectPath, error) {
	b.gotTitle, b.gotFilters = title, filters

	return b.handle, b.openErr
}

// Wait returns the fake's scripted uri or error at once, mirroring how
// the real Bus eventually answers a Response signal.
func (b *fakeBus) Wait(_ context.Context, _ dbus.ObjectPath) (string, error) {
	if b.waitErr != nil {
		return "", b.waitErr
	}

	return b.uri, nil
}

// pickerWithFake returns a Picker whose Dial hands Pick the given fake
// bus.
func pickerWithFake(b *fakeBus) *portal.Picker {
	return &portal.Picker{Dial: func(context.Context) (portal.Bus, func(), error) { return b, func() {}, nil }}
}

func TestPick_ReturnsTheLocalPathForAFileURI(t *testing.T) {
	t.Parallel()

	b := &fakeBus{uri: "file:///home/tim/vacation.jpg"}
	p := pickerWithFake(b)

	path, ok, err := p.Pick(context.Background(), "Attach a file", []portal.Filter{{Name: "Images", Globs: []string{"*.jpg"}}})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true for a successful pick")
	}
	if path != "/home/tim/vacation.jpg" {
		t.Errorf("path = %q, want /home/tim/vacation.jpg", path)
	}
	if b.gotTitle != "Attach a file" {
		t.Errorf("title = %q, want %q", b.gotTitle, "Attach a file")
	}
	if len(b.gotFilters) != 1 || b.gotFilters[0].Name != "Images" {
		t.Errorf("filters = %v, want the Images filter passed through unchanged", b.gotFilters)
	}
}

func TestPick_ReportsCancelledWithNoError(t *testing.T) {
	t.Parallel()

	b := &fakeBus{waitErr: portal.ErrCancelled}
	p := pickerWithFake(b)

	path, ok, err := p.Pick(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ok {
		t.Error("ok = true for a cancelled pick")
	}
	if path != "" {
		t.Errorf("path = %q, want empty once the user cancelled", path)
	}
}

func TestPick_FailsWhenOpenFileFails(t *testing.T) {
	t.Parallel()

	b := &fakeBus{openErr: errors.New("no portal running")}
	p := pickerWithFake(b)

	_, ok, err := p.Pick(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Pick over a failing OpenFile call succeeded")
	}
	if ok {
		t.Error("ok = true alongside an error")
	}
}

func TestPick_FailsWhenDialFails(t *testing.T) {
	t.Parallel()

	p := &portal.Picker{Dial: func(context.Context) (portal.Bus, func(), error) {
		return nil, nil, errors.New("no session bus")
	}}

	_, ok, err := p.Pick(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Pick over a failing dial succeeded")
	}
	if ok {
		t.Error("ok = true alongside an error")
	}
}

// TestPick_RefusesANonFileURI confirms Pick never trusts a scheme
// other than file:// as a local path, however the portal's reply
// claimed it: see decodeFileURI.
func TestPick_RefusesANonFileURI(t *testing.T) {
	t.Parallel()

	b := &fakeBus{uri: "http://example.com/evil"}
	p := pickerWithFake(b)

	_, ok, err := p.Pick(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Pick over a non-file uri succeeded")
	}
	if ok {
		t.Error("ok = true alongside an error")
	}
}

func TestPick_FailsWhenWaitFails(t *testing.T) {
	t.Parallel()

	b := &fakeBus{waitErr: errors.New("lost the connection")}
	p := pickerWithFake(b)

	_, ok, err := p.Pick(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Pick over a failing Wait call succeeded")
	}
	if ok {
		t.Error("ok = true alongside an error")
	}
}

// TestPick_DefaultsToDialingTheRealSessionBus confirms Pick falls back
// to dialSessionBus rather than panicking on a nil Dial func, and that
// it fails promptly rather than blocking for pickTimeout, with no
// session bus configured to reach at all: this must never depend on
// how the test runner happens to have DBUS_SESSION_BUS_ADDRESS set, or
// it could open a real dialog on whoever's desktop runs it, exactly
// what this whole package exists to avoid in a test (see
// ui-testing.md). Not parallel: t.Setenv forbids it.
func TestPick_DefaultsToDialingTheRealSessionBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent")

	p := &portal.Picker{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = p.Pick(context.Background(), "t", nil)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Pick with a nil Dial blocked rather than failing promptly with no session bus reachable")
	}
}
