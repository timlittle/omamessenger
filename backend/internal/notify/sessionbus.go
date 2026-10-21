package notify

// sessionbus.go is Bus's production implementation, driving a real
// connection to the session bus: dialing it, showing a notification,
// and matching the signals that report what happened to it, trusting
// only the notification daemon's own resolved sender.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// dbusBus is Bus's production implementation, driving a real
// connection to the session bus.
type dbusBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	sig  chan *dbus.Signal

	// sender is the notification daemon's own unique bus name, resolved
	// by watchSender once Show has a connection to resolve it from.
	// Empty until then, in which case Wait's own sender check (see
	// Wait) never matches anything: any session-bus signal whose
	// sender is unknown is refused, never trusted by default.
	sender string
}

// dialSessionBus opens a connection to the session bus, registers to
// receive the notification service's ActionInvoked and
// NotificationClosed signals, and returns it alongside a func that
// releases the connection once the caller is done waiting.
func dialSessionBus(ctx context.Context) (Bus, func(), error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("notify: connect session bus: %w", err)
	}

	if err := conn.AddMatchSignal(dbus.WithMatchInterface(notifyInterface)); err != nil {
		_ = conn.Close() // the connection was never usable; nothing left to report
		return nil, nil, fmt.Errorf("notify: watch notification signals: %w", err)
	}

	sig := make(chan *dbus.Signal, 4)
	conn.Signal(sig)

	b := &dbusBus{conn: conn, obj: conn.Object(notifyInterface, notifyPath), sig: sig}

	return b, func() { conn.RemoveSignal(sig); _ = conn.Close() }, nil
}

// Show calls org.freedesktop.Notifications.Notify and returns the id
// the service assigned the notification. It resolves the service's own
// unique bus name first (see watchSender), so Wait knows which sender
// to trust before any signal for this notification can arrive.
func (b *dbusBus) Show(appName, title, body string, actions []string, hints map[string]dbus.Variant) (uint32, error) {
	b.watchSender()

	var id uint32

	call := b.obj.Call(notifyInterface+".Notify", 0, appName, uint32(0), "", title, body, actions, hints, int32(-1))
	if err := call.Store(&id); err != nil {
		return 0, fmt.Errorf("notify: Notify call: %w", err)
	}

	return id, nil
}

// watchSender resolves org.freedesktop.Notifications' own unique bus
// name and narrows this connection's signal match to it, so Wait can
// refuse a signal from anyone else: without this, any local process on
// the session bus could send a fake ActionInvoked and make Notify
// report a click that never happened. A failure to resolve or watch it
// is never fatal - b.sender is simply left at "", which Wait's own
// check treats the same as an untrusted sender - and does nothing to
// the fakeObject-driven tests that construct a bare dbusBus with no
// real connection at all.
func (b *dbusBus) watchSender() {
	if b.conn == nil {
		return
	}

	var owner string
	if err := b.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, notifyInterface).Store(&owner); err != nil {
		return
	}

	if err := b.conn.AddMatchSignal(dbus.WithMatchInterface(notifyInterface), dbus.WithMatchSender(owner)); err != nil {
		return
	}

	b.sender = owner
}

// Wait reads signals until it sees NotificationClosed for id, returning
// whichever action key an ActionInvoked for id carried first (a shown
// notification only ever carries one action to choose), or ctx's error
// if it ends first. A signal whose Sender is not the notification
// daemon's own resolved unique bus name (see watchSender) is ignored
// outright: this is the authoritative check, never relied on through
// the match rule alone, since any local process on the session bus can
// otherwise send a fake ActionInvoked and make Notify report a click
// that never happened.
func (b *dbusBus) Wait(ctx context.Context, id uint32) (string, error) {
	action := ""

	for {
		select {
		case <-ctx.Done():
			return action, ctx.Err()
		case sig, ok := <-b.sig:
			if !ok {
				return action, errors.New("notify: signal channel closed")
			}
			if sig.Sender != b.sender {
				continue
			}

			if a, matched := matchActionInvoked(sig, id); matched {
				action = a
				continue
			}
			if matchNotificationClosed(sig, id) {
				return action, nil
			}
		}
	}
}

// matchActionInvoked reports whether sig is a well-formed ActionInvoked
// for id, and the action key it carried when it is. A signal whose body
// is too short to carry an action, or whose second argument is not a
// string, is never trusted enough to report a match: indexing its body
// without checking first is exactly the bug a spoofed signal with a
// one-element body could otherwise panic on and crash the helper.
func matchActionInvoked(sig *dbus.Signal, id uint32) (string, bool) {
	if sig.Name != sigActionInvoked || !signalIsFor(sig, id) {
		return "", false
	}

	if len(sig.Body) < 2 {
		return "", false
	}

	action, ok := sig.Body[1].(string)

	return action, ok
}

// matchNotificationClosed reports whether sig is NotificationClosed for
// id.
func matchNotificationClosed(sig *dbus.Signal, id uint32) bool {
	return sig.Name == sigNotificationClosed && signalIsFor(sig, id)
}

// signalIsFor reports whether sig's first body argument is id, the
// notification id every freedesktop notification signal leads with.
func signalIsFor(sig *dbus.Signal, id uint32) bool {
	if len(sig.Body) < 1 {
		return false
	}

	got, ok := sig.Body[0].(uint32)

	return ok && got == id
}

// serviceReachableTimeout bounds how long ServiceReachable waits for an
// answer, so the doctor report returns promptly on a desktop with no
// session bus at all.
const serviceReachableTimeout = 2 * time.Second

// ServiceReachable reports whether a notification service answers on
// the session bus, for the doctor report. It never shows a notification
// and its result is a plain state, never a path, a name or any message
// content.
func ServiceReachable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, serviceReachableTimeout)
	defer cancel()

	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }() // best effort; the check result does not depend on it

	var hasOwner bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, notifyInterface).Store(&hasOwner)

	return err == nil && hasOwner
}
