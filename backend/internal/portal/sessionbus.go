package portal

// sessionbus.go is Bus's production implementation, driving a real
// connection to the session bus: dialing it, asking
// org.freedesktop.portal.FileChooser to open a dialog, and matching the
// org.freedesktop.portal.Request.Response signal it answers with back
// to the right request, trusting only the portal's own resolved
// sender (see notify/sessionbus.go, the same pattern for a different
// signal).

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// desktopInterface and desktopPath are xdg-desktop-portal's own fixed
// bus name and object, the parent every portal interface, including
// FileChooser, is reached through.
const (
	desktopInterface = "org.freedesktop.portal.Desktop"
	desktopPath      = dbus.ObjectPath("/org/freedesktop/portal/desktop")
)

// fileChooserInterface is the portal interface OpenFile belongs to.
const fileChooserInterface = "org.freedesktop.portal.FileChooser"

// requestInterface and sigResponse name the signal every portal
// request answers with, on the object path OpenFile itself returned.
const (
	requestInterface = "org.freedesktop.portal.Request"
	sigResponse      = requestInterface + ".Response"
)

// dbusBus is Bus's production implementation, driving a real
// connection to the session bus.
type dbusBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	sig  chan *dbus.Signal

	// sender is the portal's own unique bus name, resolved by
	// watchSender once OpenFile has a connection to resolve it from.
	// Empty until then, in which case Wait's own sender check never
	// matches anything: any session-bus signal whose sender is unknown
	// is refused, never trusted by default.
	sender string
}

// dialSessionBus opens a connection to the session bus and registers to
// receive org.freedesktop.portal.Request's Response signals, returning
// it alongside a func that releases the connection once the caller is
// done waiting.
func dialSessionBus(ctx context.Context) (Bus, func(), error) {
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

	b := &dbusBus{conn: conn, obj: conn.Object(desktopInterface, desktopPath), sig: sig}

	return b, func() { conn.RemoveSignal(sig); _ = conn.Close() }, nil
}

// portalFilterRule is one glob or mime-type rule within a named
// filter: Kind 0 means Pattern is a glob ("*.jpg"), matching the
// a(us) xdg-desktop-portal expects inside each named filter's own
// array.
type portalFilterRule struct {
	Kind    uint32
	Pattern string
}

// portalFilterGroup is one named group of rules, matching the
// a(sa(us)) OpenFile's "filters" option expects.
type portalFilterGroup struct {
	Name  string
	Rules []portalFilterRule
}

// toPortalFilters converts Filter, this package's own plain type, into
// the a(sa(us)) shape xdg-desktop-portal's FileChooser.OpenFile
// expects for its "filters" option, every rule a glob (kind 0).
func toPortalFilters(filters []Filter) []portalFilterGroup {
	groups := make([]portalFilterGroup, 0, len(filters))
	for _, f := range filters {
		rules := make([]portalFilterRule, 0, len(f.Globs))
		for _, glob := range f.Globs {
			rules = append(rules, portalFilterRule{Kind: 0, Pattern: glob})
		}
		groups = append(groups, portalFilterGroup{Name: f.Name, Rules: rules})
	}

	return groups
}

// OpenFile calls org.freedesktop.portal.FileChooser.OpenFile and
// returns the request object path its Response signal will carry. It
// resolves the portal's own unique bus name first (see watchSender),
// so Wait knows which sender to trust before any signal for this
// request can arrive.
func (b *dbusBus) OpenFile(_ context.Context, title string, filters []Filter) (dbus.ObjectPath, error) {
	b.watchSender()

	options := map[string]dbus.Variant{
		"multiple": dbus.MakeVariant(false),
		"modal":    dbus.MakeVariant(true),
	}
	if len(filters) > 0 {
		options["filters"] = dbus.MakeVariant(toPortalFilters(filters))
	}

	var handle dbus.ObjectPath

	call := b.obj.Call(fileChooserInterface+".OpenFile", 0, "", title, options)
	if err := call.Store(&handle); err != nil {
		return "", fmt.Errorf("portal: OpenFile call: %w", err)
	}

	return handle, nil
}

// watchSender resolves xdg-desktop-portal's own unique bus name and
// narrows this connection's signal match to it, so Wait can refuse a
// signal from anyone else: without this, any local process on the
// session bus could send a fake Response and make Pick report a file
// the user never actually chose. A failure to resolve or watch it is
// never fatal - b.sender is simply left at "", which Wait's own check
// treats the same as an untrusted sender - and does nothing to the
// fakeObject-driven tests that construct a bare dbusBus with no real
// connection at all.
func (b *dbusBus) watchSender() {
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

// Wait reads signals until it sees a Response for path, returning the
// chosen file's own file:// URI, ErrCancelled once the user cancels, or
// ctx's error if it ends first. A signal whose Sender is not the
// portal's own resolved unique bus name, or whose Path is not path, is
// ignored outright (see watchSender).
func (b *dbusBus) Wait(ctx context.Context, path dbus.ObjectPath) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case sig, ok := <-b.sig:
			if !ok {
				return "", errors.New("portal: signal channel closed")
			}
			if sig.Sender != b.sender || sig.Path != path {
				continue
			}

			if matched, uri, err := matchResponse(sig); matched {
				return uri, err
			}
		}
	}
}

// matchResponse reports whether sig is a well-formed
// org.freedesktop.portal.Request.Response, and the chosen file's own
// uri or error when it is. A signal whose body is too short, or whose
// arguments are not the types the portal's own contract promises, is
// never trusted enough to index further: that is exactly the bug a
// spoofed or malformed signal could otherwise panic on and crash the
// helper (see notify/sessionbus.go's matchActionInvoked, the same
// discipline for a different signal).
func matchResponse(sig *dbus.Signal) (matched bool, uri string, err error) {
	if sig.Name != sigResponse {
		return false, "", nil
	}
	if len(sig.Body) < 2 {
		return true, "", errors.New("portal: malformed response signal")
	}

	code, ok := sig.Body[0].(uint32)
	if !ok {
		return true, "", errors.New("portal: malformed response code")
	}
	if code != 0 {
		return true, "", ErrCancelled
	}

	results, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return true, "", errors.New("portal: malformed response results")
	}

	uris, ok := results["uris"]
	if !ok {
		return true, "", ErrCancelled
	}

	paths, ok := uris.Value().([]string)
	if !ok || len(paths) == 0 {
		return true, "", ErrCancelled
	}

	return true, paths[0], nil
}
