// Package notify shows desktop notifications with a direct call to
// org.freedesktop.Notifications on the session bus, which Omarchy's
// notification daemon already answers. It never shells out to a helper
// process: a notification's title, body and conversation id travel only
// as D-Bus call arguments, so no other local user can read them from a
// process's command line.
package notify

import (
	"context"
	"strings"
	"sync"
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

// MaxConcurrentNotifications bounds how many notifications a Desktop
// shows at once, and so how many goroutines and session-bus
// connections it can have open at once, each living up to
// notifyTimeout: a burst of incoming messages (reconnecting after
// being offline, say) must never be able to open an unbounded number
// of either. A notification beyond this many already showing is
// dropped at once rather than queued (see acquire): a notification is
// a transient hint, never something worth making a later one wait
// behind.
const MaxConcurrentNotifications = 8

// maxTitleRunes and maxBodyRunes bound a notification's title and
// body. Generous for any real conversation name or message preview -
// Omarchy's own notification daemon already wraps long text to a few
// lines on screen - while keeping a single crafted message from
// growing the D-Bus call this package makes, and what the daemon then
// has to render, without bound.
const (
	maxTitleRunes = 200
	maxBodyRunes  = 2000
)

// truncateRunes returns s unchanged if it already has at most max
// runes, or its first max runes otherwise. Converting to []rune first,
// rather than slicing the string's bytes directly, means the cut
// always falls on a rune boundary, never partway through a multi-byte
// character.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}

	return string(runes[:max])
}

// Logger writes one diagnostic line, such as a recovered panic's own
// category, to the helper's stderr. *log.Logger already satisfies it,
// so main.go wires the same logger every other package uses, with
// nothing special to construct.
type Logger interface {
	Printf(format string, v ...any)
}

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

	// Log records a panic recovered from a notification's own goroutine
	// (see Notify), as a diagnostic line carrying no notification
	// content at all. Nil means nowhere to log it; the panic is still
	// recovered and swallowed either way.
	Log Logger

	// mu guards sem's own lazy creation (see acquire): Desktop is often
	// constructed as a bare struct literal, so the semaphore cannot be
	// set up by a constructor this package has no other use for.
	mu  sync.Mutex
	sem chan struct{}
}

// acquire reserves one of this Desktop's MaxConcurrentNotifications
// concurrent slots, creating the semaphore on first use, and reports
// whether it got one: false means that many are already showing, and
// the caller must drop this notification rather than wait behind them.
func (d *Desktop) acquire() bool {
	d.mu.Lock()
	if d.sem == nil {
		d.sem = make(chan struct{}, MaxConcurrentNotifications)
	}
	sem := d.sem
	d.mu.Unlock()

	select {
	case sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// release frees the slot a successful acquire reserved.
func (d *Desktop) release() {
	d.mu.Lock()
	sem := d.sem
	d.mu.Unlock()

	<-sem
}

// Notify shows title and body as a notification carrying
// conversationID, and waits for it in its own goroutine so the caller
// never blocks. A missing or failing notification service is ignored: a
// notification is never worth failing a message over. One already
// showing beyond MaxConcurrentNotifications is dropped at once instead
// (see acquire); title and body are bounded and cut on a rune boundary
// (see truncateRunes) before anything else happens to them.
func (d *Desktop) Notify(title, body, conversationID string) {
	if !d.acquire() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)

	dial := d.Dial
	if dial == nil {
		dial = dialSessionBus
	}

	title, body = truncateRunes(title, maxTitleRunes), truncateRunes(body, maxBodyRunes)

	go func() {
		defer cancel()
		defer d.release()
		defer d.recoverPanic()
		d.run(ctx, dial, title, body, conversationID)
	}()
}

// recoverPanic stops a panic inside a notification's own goroutine
// from crashing the helper: one malformed signal from the notification
// daemon, or a bug reached only through it, must never bring down every
// other notification along with it, let alone the helper itself (see
// errors.md: recover at the protocol boundary). Only the recovered
// value's own type is logged, never its content, since Go lets any
// value be panicked with and this package must never assume one
// carries nothing sensitive.
func (d *Desktop) recoverPanic() {
	r := recover()
	if r == nil || d.Log == nil {
		return
	}

	d.Log.Printf("notify: recovered from a panic in a notification's own goroutine: %T", r)
}

// run dials the bus, shows the notification, waits for it to close and
// reports a click if the user chose the default action. It treats a
// dial, show or wait failure as nothing to report rather than a crash:
// see Notify, which releases ctx and the bus connection once run
// returns.
func (d *Desktop) run(ctx context.Context, dial func(context.Context) (Bus, func(), error), title, body, conversationID string) {
	b, release, err := dial(ctx)
	if err != nil {
		return
	}
	defer release()

	id, err := b.Show("OmaMessenger", title, escapeBody(body), []string{defaultAction, "Open"}, map[string]dbus.Variant{
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

// escapeBody neutralizes the markup characters a notification server
// that advertises the freedesktop "body-markup" capability - Omarchy's
// own daemon does, hyperlinks included - interprets in a notification
// body. A message's own text must never be read as a "<a href>" link
// or a "<b>" tag just because it happens to contain the same
// characters HTML markup uses; the notification spec's body-markup
// subset needs only '&', '<' and '>' escaped, so that is all this
// does. The summary (title) is never escaped: the spec requires it be
// shown as plain text, with no markup interpreted at all.
func escapeBody(body string) string {
	body = strings.ReplaceAll(body, "&", "&amp;")
	body = strings.ReplaceAll(body, "<", "&lt;")
	body = strings.ReplaceAll(body, ">", "&gt;")

	return body
}
