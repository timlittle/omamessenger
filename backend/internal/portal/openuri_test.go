// Opener's Dial field replaces a real session bus connection, so every
// test here runs a hand-written fake OpenBus instead of reaching one,
// and never opens a real link or file through a real portal.
package portal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/timlittle/omamessenger/backend/internal/portal"
)

// fakeOpenBus is a hand-written stand-in for portal.OpenBus. It
// records OpenURI's and OpenFile's own arguments and lets a test
// script Wait's own reply, without ever reaching a real session bus.
type fakeOpenBus struct {
	gotURI  string
	gotFile *os.File

	openErr error
	handle  dbus.ObjectPath

	waitErr error
}

// OpenURI records its argument and returns the fake's scripted handle
// and error.
func (b *fakeOpenBus) OpenURI(_ context.Context, uri string) (dbus.ObjectPath, error) {
	b.gotURI = uri

	return b.handle, b.openErr
}

// OpenFile records its argument and returns the fake's scripted handle
// and error.
func (b *fakeOpenBus) OpenFile(_ context.Context, f *os.File) (dbus.ObjectPath, error) {
	b.gotFile = f

	return b.handle, b.openErr
}

// Wait returns the fake's scripted error at once, mirroring how the
// real OpenBus eventually answers a Response signal.
func (b *fakeOpenBus) Wait(_ context.Context, _ dbus.ObjectPath) error {
	return b.waitErr
}

// openerWithFake returns an Opener whose Dial hands every call the
// given fake bus.
func openerWithFake(b *fakeOpenBus) *portal.Opener {
	return &portal.Opener{Dial: func(context.Context) (portal.OpenBus, func(), error) { return b, func() {}, nil }}
}

func TestOpen_CallsOpenURIWithTheGivenURI(t *testing.T) {
	t.Parallel()

	b := &fakeOpenBus{}
	o := openerWithFake(b)

	if err := o.Open(context.Background(), "https://example.com/reset?token=secret"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if b.gotURI != "https://example.com/reset?token=secret" {
		t.Errorf("gotURI = %q, want the uri passed through unchanged", b.gotURI)
	}
}

func TestOpen_FailsWhenDialFails(t *testing.T) {
	t.Parallel()

	o := &portal.Opener{Dial: func(context.Context) (portal.OpenBus, func(), error) {
		return nil, nil, errors.New("no session bus")
	}}

	if err := o.Open(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Open over a failing dial succeeded")
	}
}

func TestOpen_FailsWhenOpenURIFails(t *testing.T) {
	t.Parallel()

	b := &fakeOpenBus{openErr: errors.New("no portal running")}
	o := openerWithFake(b)

	if err := o.Open(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Open over a failing OpenURI call succeeded")
	}
}

func TestOpen_FailsWhenWaitFails(t *testing.T) {
	t.Parallel()

	b := &fakeOpenBus{waitErr: errors.New("request was not completed")}
	o := openerWithFake(b)

	if err := o.Open(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Open over a failing Wait call succeeded")
	}
}

// TestOpen_DefaultsToDialingTheRealSessionBus confirms Open falls back
// to dialOpenBus rather than panicking on a nil Dial func, and that it
// fails promptly rather than blocking for openTimeout, with no session
// bus configured to reach at all: this must never depend on how the
// test runner happens to have DBUS_SESSION_BUS_ADDRESS set, or it
// could reach a real portal on whoever's desktop runs it (see
// ui-testing.md). Not parallel: t.Setenv forbids it.
func TestOpen_DefaultsToDialingTheRealSessionBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent")

	o := &portal.Opener{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = o.Open(context.Background(), "https://example.com")
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Open with a nil Dial blocked rather than failing promptly with no session bus reachable")
	}
}

func TestOpenFile_OpensTheFileAndHandsItToOpenFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("jpeg bytes"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	b := &fakeOpenBus{}
	o := openerWithFake(b)

	if err := o.OpenFile(context.Background(), path); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if b.gotFile == nil {
		t.Fatal("OpenFile never reached OpenBus.OpenFile with a file")
	}
	if got := b.gotFile.Name(); got != path {
		t.Errorf("gotFile.Name() = %q, want %q", got, path)
	}
}

func TestOpenFile_FailsWhenTheFileDoesNotExist(t *testing.T) {
	t.Parallel()

	b := &fakeOpenBus{}
	o := openerWithFake(b)

	if err := o.OpenFile(context.Background(), filepath.Join(t.TempDir(), "missing.jpg")); err == nil {
		t.Fatal("OpenFile over a missing path succeeded")
	}
	if b.gotFile != nil {
		t.Error("OpenFile reached OpenBus.OpenFile despite the local open failing")
	}
}

func TestOpenFile_FailsWhenOpenFileCallFails(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("jpeg bytes"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	b := &fakeOpenBus{openErr: errors.New("no portal running")}
	o := openerWithFake(b)

	if err := o.OpenFile(context.Background(), path); err == nil {
		t.Fatal("OpenFile over a failing OpenFile call succeeded")
	}
}
