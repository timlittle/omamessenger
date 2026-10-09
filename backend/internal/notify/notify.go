// Package notify shows desktop notifications with a direct call to
// org.freedesktop.Notifications on the session bus, which Omarchy's
// notification daemon already answers. It never shells out to a helper
// process: a notification's title, body and conversation id travel only
// as D-Bus call arguments, so no other local user can read them from a
// process's command line.
package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// notifyInterface is the freedesktop notification service's own bus
// name, object path and interface, all the same string by convention.
const notifyInterface = "org.freedesktop.Notifications"

// notifyPath is the object the Notify method and its signals live on.
const notifyPath = dbus.ObjectPath("/org/freedesktop/Notifications")

// sigActionInvoked and sigNotificationClosed are the two signals a shown
// notification sends back: which action (if any) the user chose, and
// that it has since closed.
const (
	sigActionInvoked      = notifyInterface + ".ActionInvoked"
	sigNotificationClosed = notifyInterface + ".NotificationClosed"
)

// defaultAction is the freedesktop "default" action id: the daemon
// treats it as the user clicking the notification body itself, rather
// than a named button.
const defaultAction = "default"

// notifyTimeout bounds how long Notify's goroutine waits for the
// notification to close, so that goroutine cannot leak forever if a
// notification is somehow never closed. It is not meant to cut a real
// notification short.
const notifyTimeout = 5 * time.Minute

// Bus is the one seam between Desktop and a real D-Bus session bus
// connection: showing a notification and waiting for its own
// ActionInvoked or NotificationClosed signal. dialSessionBus is the
// production implementation, over a real *dbus.Conn; tests use a
// hand-written fake instead of a real session bus.
type Bus interface {
	// Show calls the Notify method and returns the id the service
	// assigned the notification, used to match the signals that
	// report what happened to it.
	Show(appName, title, body string, actions []string, hints map[string]dbus.Variant) (uint32, error)

	// Wait blocks until the service reports id closed, returning the
	// action key chosen beforehand ("" when there was none), or until
	// ctx ends.
	Wait(ctx context.Context, id uint32) (action string, err error)
}

// Desktop shows notifications over D-Bus and reports when the user
// clicks one.
type Desktop struct {
	// Dial opens the Bus Notify talks to. Nil uses dialSessionBus;
	// tests replace it with a fake so nothing reaches a real session
	// bus.
	Dial func(ctx context.Context) (Bus, func(), error)

	// Click is called with a notification's conversation id when the
	// user chooses its "default" action: clicking the notification
	// itself, rather than closing or ignoring it. Nil means clicks are
	// not reported.
	Click func(conversationID string)
}

// Notify shows title and body as a notification carrying
// conversationID, and waits for it in its own goroutine so the caller
// never blocks. A missing or failing notification service is ignored: a
// notification is never worth failing a message over.
func (d Desktop) Notify(title, body, conversationID string) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)

	dial := d.Dial
	if dial == nil {
		dial = dialSessionBus
	}

	go func() {
		defer cancel()
		d.run(ctx, dial, title, body, conversationID)
	}()
}

// run dials the bus, shows the notification, waits for it to close and
// reports a click if the user chose the default action. It treats a
// dial, show or wait failure as nothing to report rather than a crash:
// see Notify, which releases ctx and the bus connection once run
// returns.
func (d Desktop) run(ctx context.Context, dial func(context.Context) (Bus, func(), error), title, body, conversationID string) {
	b, release, err := dial(ctx)
	if err != nil {
		return
	}
	defer release()

	id, err := b.Show("OmaMessenger", title, body, []string{defaultAction, "Open"}, map[string]dbus.Variant{
		"category": dbus.MakeVariant("im.received"),
	})
	if err != nil {
		return
	}

	action, err := b.Wait(ctx, id)
	if err != nil {
		return
	}

	if action == defaultAction && d.Click != nil {
		d.Click(conversationID)
	}
}

// dbusBus is Bus's production implementation, driving a real
// connection to the session bus.
type dbusBus struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	sig  chan *dbus.Signal
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
// the service assigned the notification.
func (b *dbusBus) Show(appName, title, body string, actions []string, hints map[string]dbus.Variant) (uint32, error) {
	var id uint32

	call := b.obj.Call(notifyInterface+".Notify", 0, appName, uint32(0), "", title, body, actions, hints, int32(-1))
	if err := call.Store(&id); err != nil {
		return 0, fmt.Errorf("notify: Notify call: %w", err)
	}

	return id, nil
}

// Wait reads signals until it sees NotificationClosed for id, returning
// whichever action key an ActionInvoked for id carried first (a shown
// notification only ever carries one action to choose), or ctx's error
// if it ends first.
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

// matchActionInvoked reports whether sig is ActionInvoked for id, and
// the action key it carried when it is.
func matchActionInvoked(sig *dbus.Signal, id uint32) (string, bool) {
	if sig.Name != sigActionInvoked || !signalIsFor(sig, id) {
		return "", false
	}

	action, _ := sig.Body[1].(string)

	return action, true
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
