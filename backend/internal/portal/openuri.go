package portal

// openuri.go opens a URL or a local file through
// org.freedesktop.portal.OpenURI, out of this process entirely, so
// neither ever becomes a command-line argument of a process this
// helper starts: a process started with xdg-open (or any browser it
// in turn launches) carries its argument in full in its own argv,
// which every other local user can read through /proc/<pid>/cmdline
// (see docs/decisions.md). A clicked link can carry something as
// sensitive as a password-reset token or a meeting password, so that
// leak matters even for the few seconds the process lives.
//
// Opener follows the same dial/release lifecycle, and the same
// request-path-and-sender discipline, as Picker/Bus above (see
// sessionbus.go's watchSender and matchResponse): a Response is
// trusted only once it answers this call's own request path and comes
// from the portal's own resolved unique bus name.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

// openTimeout bounds how long Open and OpenFile wait for the portal to
// answer, so a forgotten response can never leak the request, or the
// goroutine the server's own dispatch runs it on, forever.
const openTimeout = 30 * time.Second

// openURIInterface is the portal interface OpenURI and OpenFile belong
// to.
const openURIInterface = "org.freedesktop.portal.OpenURI"

// OpenBus is the one seam between Opener and a real D-Bus session bus
// connection: making the OpenURI portal's own call and waiting for its
// Request.Response signal. dialOpenBus is the production
// implementation; tests use a hand-written fake instead, so no test
// ever reaches a real portal.
type OpenBus interface {
	// OpenURI calls OpenURI.OpenURI and returns the request object
	// path its Response signal will carry.
	OpenURI(ctx context.Context, uri string) (dbus.ObjectPath, error)

	// OpenFile calls OpenURI.OpenFile, handing the portal f's own open
	// file descriptor rather than its path, and returns the request
	// object path its Response signal will carry.
	OpenFile(ctx context.Context, f *os.File) (dbus.ObjectPath, error)

	// Wait blocks until a Response signal for path arrives, returning
	// nil once the portal reports it opened the target, an error
	// otherwise, or ctx's error if it ends first.
	Wait(ctx context.Context, path dbus.ObjectPath) error
}

// Opener opens a URI or a local file through the portal's own OpenURI
// interface.
type Opener struct {
	// Dial opens the OpenBus Open and OpenFile talk to. Nil uses
	// dialOpenBus; tests replace it with a fake so nothing reaches a
	// real session bus.
	Dial func(ctx context.Context) (OpenBus, func(), error)
}

// Open asks the portal to open uri with the user's own default
// handler for its scheme, typically a browser.
func (o *Opener) Open(ctx context.Context, uri string) error {
	return o.call(ctx, func(ctx context.Context, bus OpenBus) (dbus.ObjectPath, error) {
		return bus.OpenURI(ctx, uri)
	})
}

// OpenFile asks the portal to open the local file at path with the
// user's own default handler for its kind, handing the portal an open
// file descriptor rather than path itself: OpenURI.OpenFile's own
// contract for a local file, and never a path a spawned process's own
// argv would carry.
func (o *Opener) OpenFile(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("portal: open file: %w", err)
	}
	defer func() { _ = f.Close() }() // the portal call reads its own duplicate of the descriptor; this copy is no longer needed either way

	return o.call(ctx, func(ctx context.Context, bus OpenBus) (dbus.ObjectPath, error) {
		return bus.OpenFile(ctx, f)
	})
}

// call runs the shared dial/request/wait/release sequence behind Open
// and OpenFile, so neither duplicates it.
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

// dbusOpenBus is OpenBus's production implementation, the OpenURI
// counterpart to dbusBus (sessionbus.go): the same dial, sender
// resolution and Request.Response matching discipline, applied to
// org.freedesktop.portal.OpenURI's own OpenURI and OpenFile calls
// instead of FileChooser's OpenFile.
type dbusOpenBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	sig  chan *dbus.Signal

	// sender is the portal's own unique bus name, resolved by
	// watchSender once OpenURI or OpenFile has a connection to resolve
	// it from. Empty until then, in which case Wait's own sender check
	// never matches anything.
	sender string
}

// dialOpenBus opens a connection to the session bus and registers to
// receive org.freedesktop.portal.Request's Response signals, the
// OpenURI counterpart to dialSessionBus.
func dialOpenBus(ctx context.Context) (OpenBus, func(), error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("portal: connect session bus: %w", err)
	}

	if err := conn.AddMatchSignal(dbus.WithMatchInterface(requestInterface)); err != nil {
		_ = conn.Close() // the connection was never usable; nothing left to report
		return nil, nil, fmt.Errorf("portal: watch request signals: %w", err)
	}

	sig := make(chan *dbus.Signal, 4)
	conn.Signal(sig)

	b := &dbusOpenBus{conn: conn, obj: conn.Object(desktopInterface, desktopPath), sig: sig}

	return b, func() { conn.RemoveSignal(sig); _ = conn.Close() }, nil
}

// OpenURI calls org.freedesktop.portal.OpenURI.OpenURI and returns the
// request object path its Response signal will carry.
func (b *dbusOpenBus) OpenURI(_ context.Context, uri string) (dbus.ObjectPath, error) {
	b.watchSender()

	var handle dbus.ObjectPath

	call := b.obj.Call(openURIInterface+".OpenURI", 0, "", uri, map[string]dbus.Variant{})
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("portal: OpenURI call: %w", err)
	}

	return handle, nil
}

// OpenFile calls org.freedesktop.portal.OpenURI.OpenFile with f's own
// file descriptor and returns the request object path its Response
// signal will carry.
func (b *dbusOpenBus) OpenFile(_ context.Context, f *os.File) (dbus.ObjectPath, error) {
	b.watchSender()

	var handle dbus.ObjectPath

	call := b.obj.Call(openURIInterface+".OpenFile", 0, "", dbus.UnixFD(f.Fd()), map[string]dbus.Variant{})
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("portal: OpenFile call: %w", err)
	}

	return handle, nil
}

// watchSender resolves xdg-desktop-portal's own unique bus name and
// narrows this connection's signal match to it, the same reasoning as
// dbusBus.watchSender (sessionbus.go): without this, any local process
// on the session bus could send a fake Response and make Wait report
// success for an open that never happened. A failure to resolve or
// watch it is never fatal - b.sender is simply left at "", which
// Wait's own check treats the same as an untrusted sender.
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

// Wait reads signals until it sees a Response for path, returning nil
// once the portal reports success, an error otherwise, or ctx's error
// if it ends first. A signal whose Sender is not the portal's own
// resolved unique bus name, or whose Path is not path, is ignored
// outright (see watchSender).
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

// matchOpenResponse reports whether sig is a well-formed
// org.freedesktop.portal.Request.Response for an OpenURI call, and the
// error it carries when it is: nil for a successful open (code 0), an
// error for anything else. A signal whose body is too short, or whose
// response code is not the type the portal's own contract promises, is
// never trusted enough to index further (see matchResponse in
// sessionbus.go, the same discipline for FileChooser's own Response).
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
