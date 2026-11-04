package portal

// openuri.go opens a URL or local file through
// org.freedesktop.portal.OpenURI instead of spawning xdg-open, so the
// target never appears in a process argv another local user could
// read from /proc (see docs/decisions.md).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

// openTimeout bounds how long Open and OpenFile wait for the portal to answer.
const openTimeout = 30 * time.Second

// openURIInterface is the portal interface OpenURI and OpenFile belong to.
const openURIInterface = "org.freedesktop.portal.OpenURI"

// OpenBus is the D-Bus connection Opener uses, faked in tests. It needs
// three methods because each portal call is a request plus a signal wait.
type OpenBus interface {
	// OpenURI calls OpenURI.OpenURI and returns the request's object path.
	OpenURI(ctx context.Context, uri string) (dbus.ObjectPath, error)

	// OpenFile calls OpenURI.OpenFile with f's descriptor and returns the request's object path.
	OpenFile(ctx context.Context, f *os.File) (dbus.ObjectPath, error)

	// Wait blocks until the Response signal for path arrives.
	Wait(ctx context.Context, path dbus.ObjectPath) error
}

// Opener opens a URI or a local file through the portal's OpenURI interface.
type Opener struct {
	// Dial opens the OpenBus Open and OpenFile talk to. Nil uses dialOpenBus.
	Dial func(ctx context.Context) (OpenBus, func(), error)
}

// Open asks the portal to open uri with the user's own default
// handler for its scheme, typically a browser.
func (o *Opener) Open(ctx context.Context, uri string) error {
	return o.call(ctx, func(ctx context.Context, bus OpenBus) (dbus.ObjectPath, error) {
		return bus.OpenURI(ctx, uri)
	})
}

// OpenFile asks the portal to open the local file at path, passing an
// open file descriptor rather than the path itself.
func (o *Opener) OpenFile(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("portal: open file: %w", err)
	}
	defer func() { _ = f.Close() }() // the portal reads its own duplicate of the descriptor

	return o.call(ctx, func(ctx context.Context, bus OpenBus) (dbus.ObjectPath, error) {
		return bus.OpenFile(ctx, f)
	})
}

// call dials the bus, sends request and waits for its response.
func (o *Opener) call(ctx context.Context, request func(context.Context, OpenBus) (dbus.ObjectPath, error)) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()

	dial := o.Dial
	if dial == nil {
		dial = dialOpenBus
	}

	bus, release, err := dial(ctx)
	if err != nil {
		return fmt.Errorf("portal: dial session bus: %w", err)
	}
	defer release()

	reqPath, err := request(ctx, bus)
	if err != nil {
		return fmt.Errorf("portal: open call: %w", err)
	}

	return bus.Wait(ctx, reqPath)
}

// dbusOpenBus is OpenBus over a real session bus connection.
type dbusOpenBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	sig  chan *dbus.Signal

	// sender is the portal's unique bus name; empty matches no signal.
	sender string
}

// dialOpenBus opens a session bus connection and registers to receive
// org.freedesktop.portal.Request's Response signals.
func dialOpenBus(ctx context.Context) (OpenBus, func(), error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("portal: connect session bus: %w", err)
	}

	if err := conn.AddMatchSignal(dbus.WithMatchInterface(requestInterface)); err != nil {
		_ = conn.Close() // never usable; nothing left to report
		return nil, nil, fmt.Errorf("portal: watch request signals: %w", err)
	}

	sig := make(chan *dbus.Signal, 4)
	conn.Signal(sig)

	b := &dbusOpenBus{conn: conn, obj: conn.Object(desktopInterface, desktopPath), sig: sig}

	return b, func() { conn.RemoveSignal(sig); _ = conn.Close() }, nil
}

// OpenURI calls OpenURI.OpenURI and returns the request's object path.
func (b *dbusOpenBus) OpenURI(_ context.Context, uri string) (dbus.ObjectPath, error) {
	b.watchSender()

	var handle dbus.ObjectPath

	call := b.obj.Call(openURIInterface+".OpenURI", 0, "", uri, map[string]dbus.Variant{})
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("portal: OpenURI call: %w", err)
	}

	return handle, nil
}

// OpenFile calls OpenURI.OpenFile with f's descriptor and returns the request's object path.
func (b *dbusOpenBus) OpenFile(_ context.Context, f *os.File) (dbus.ObjectPath, error) {
	b.watchSender()

	var handle dbus.ObjectPath

	call := b.obj.Call(openURIInterface+".OpenFile", 0, "", dbus.UnixFD(f.Fd()), map[string]dbus.Variant{})
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("portal: OpenFile call: %w", err)
	}

	return handle, nil
}

// watchSender narrows the signal match to the portal's unique bus name,
// so another local process cannot spoof a Response.
func (b *dbusOpenBus) watchSender() {
	if b.conn == nil {
		return
	}

	var owner string
	if err := b.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, desktopInterface).Store(&owner); err != nil {
		return
	}

	if err := b.conn.AddMatchSignal(dbus.WithMatchInterface(requestInterface), dbus.WithMatchSender(owner)); err != nil {
		return
	}

	b.sender = owner
}

// Wait blocks until the portal's Response for path arrives or ctx ends.
func (b *dbusOpenBus) Wait(ctx context.Context, path dbus.ObjectPath) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig, ok := <-b.sig:
			if !ok {
				return errors.New("portal: signal channel closed")
			}
			if sig.Sender != b.sender || sig.Path != path {
				continue
			}

			if matched, err := matchOpenResponse(sig); matched {
				return err
			}
		}
	}
}

// matchOpenResponse reports whether sig is a Response and, if so, its error.
func matchOpenResponse(sig *dbus.Signal) (matched bool, err error) {
	if sig.Name != sigResponse {
		return false, nil
	}
	if len(sig.Body) < 1 {
		return true, errors.New("portal: malformed response signal")
	}

	code, ok := sig.Body[0].(uint32)
	if !ok {
		return true, errors.New("portal: malformed response code")
	}
	if code != 0 {
		return true, errors.New("portal: request was not completed")
	}

	return true, nil
}
