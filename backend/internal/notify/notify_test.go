// Desktop's Dial field replaces a real session bus connection, so every
// test here runs a hand-written fake Bus instead of reaching one, and
// can run in parallel.
package notify_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/godbus/dbus/v5"

	"github.com/timlittle/omamessenger/backend/internal/notify"
)

// fakeBus is a hand-written stand-in for notify.Bus. It records Show's
// arguments and lets a test script the signals Wait sees, without ever
// reaching a real session bus.
type fakeBus struct {
	mu sync.Mutex

	gotAppName string
	gotTitle   string
	gotBody    string
	gotActions []string
	gotHints   map[string]dbus.Variant

	showErr error
	id      uint32

	signals chan string // one entry per ActionInvoked action key seen before close; "" closes with no action
	waitErr error
}

// Show records its arguments and returns the fake's scripted id and
// error.
func (b *fakeBus) Show(appName, title, body string, actions []string, hints map[string]dbus.Variant) (uint32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.gotAppName, b.gotTitle, b.gotBody, b.gotActions, b.gotHints = appName, title, body, actions, hints

	return b.id, b.showErr
}

// Wait reads from signals until it is closed or ctx ends, mirroring how
// the real Bus waits for NotificationClosed after an optional
// ActionInvoked.
func (b *fakeBus) Wait(ctx context.Context, _ uint32) (string, error) {
	if b.waitErr != nil {
		return "", b.waitErr
	}

	select {
	case action := <-b.signals:
		return action, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// args returns Show's recorded arguments under the fake's lock, so a
// test can read them once the goroutine that called Show has finished.
func (b *fakeBus) args() (appName, title, body string, actions []string, hints map[string]dbus.Variant) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.gotAppName, b.gotTitle, b.gotBody, b.gotActions, b.gotHints
}

// desktopWithFake returns a Desktop whose Dial hands Notify the given
// fake bus, and a channel fed by Click.
func desktopWithFake(b *fakeBus) (*notify.Desktop, <-chan string) {
	clicks := make(chan string, 1)
	d := &notify.Desktop{
		Dial:  func(context.Context) (notify.Bus, func(), error) { return b, func() {}, nil },
		Click: func(conversationID string) { clicks <- conversationID },
	}

	return d, clicks
}

// waitForClick returns the conversation id Click was called with, or
// fails the test if it never was.
func waitForClick(t *testing.T, clicks <-chan string) string {
	t.Helper()

	select {
	case id := <-clicks:
		return id
	case <-time.After(10 * time.Second):
		t.Fatal("click was never reported")
		return ""
	}
}

// assertNoClick fails the test if clicks receives anything within a
// window well beyond how long the fake needs to answer.
func assertNoClick(t *testing.T, clicks <-chan string) {
	t.Helper()

	select {
	case id := <-clicks:
		t.Fatalf("unexpected click for conversation %q", id)
	case <-time.After(200 * time.Millisecond):
	}
}

// closedWithoutAction scripts a fake bus whose Wait reports the
// notification closed with no action chosen.
func closedWithoutAction() *fakeBus {
	b := &fakeBus{signals: make(chan string, 1)}
	b.signals <- ""

	return b
}

// closedWithDefaultAction scripts a fake bus whose Wait reports the
// default action was chosen before the notification closed.
func closedWithDefaultAction() *fakeBus {
	b := &fakeBus{signals: make(chan string, 1)}
	b.signals <- "default"

	return b
}

func TestNotify_CallsShowWithTitleBodyAndDefaultAction(t *testing.T) {
	t.Parallel()

	b := closedWithoutAction()
	d, clicks := desktopWithFake(b)

	d.Notify("Alex · Work", "New message", "conv-1")
	assertNoClick(t, clicks)

	appName, title, body, actions, hints := b.args()
	if appName != "OmaMessenger" {
		t.Errorf("app name = %q, want OmaMessenger", appName)
	}
	if title != "Alex · Work" || body != "New message" {
		t.Errorf("title/body = %q/%q, want %q/%q", title, body, "Alex · Work", "New message")
	}
	if len(actions) != 2 || actions[0] != "default" {
		t.Errorf("actions = %v, want a default action first", actions)
	}
	if got, ok := hints["category"]; !ok || got.Value() != "im.received" {
		t.Errorf("category hint = %v, want im.received", hints["category"])
	}
}

// TestNotify_EscapesMarkupCharactersInTheBody confirms a message's own
// text cannot be read as markup by a notification server that
// advertises body-markup and hyperlinks, such as Omarchy's: '&', '<'
// and '>' are escaped in the body before it ever reaches Show, and the
// title (summary) is passed through unchanged, since the notification
// spec requires it be shown as plain text.
func TestNotify_EscapesMarkupCharactersInTheBody(t *testing.T) {
	t.Parallel()

	b := closedWithoutAction()
	d, clicks := desktopWithFake(b)

	d.Notify("Alex <b>", `<b>click here</b> & a<b x="y"> &amp; done`, "conv-1")
	assertNoClick(t, clicks)

	_, title, body, _, _ := b.args()
	if title != "Alex <b>" {
		t.Errorf("title = %q, want it passed through unescaped: the summary is plain text per the notification spec", title)
	}

	want := "&lt;b&gt;click here&lt;/b&gt; &amp; a&lt;b x=\"y\"&gt; &amp;amp; done"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestNotify_ReportsAClickWithTheConversationID(t *testing.T) {
	t.Parallel()

	d, clicks := desktopWithFake(closedWithDefaultAction())

	d.Notify("title", "body", "conv-42")

	if got := waitForClick(t, clicks); got != "conv-42" {
		t.Errorf("click reported conversation %q, want %q", got, "conv-42")
	}
}

func TestNotify_IgnoresANotificationClosedWithoutAnAction(t *testing.T) {
	t.Parallel()

	d, clicks := desktopWithFake(closedWithoutAction())

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

func TestNotify_IgnoresAnUnrelatedAction(t *testing.T) {
	t.Parallel()

	b := &fakeBus{signals: make(chan string, 1)}
	b.signals <- "ignored-action"
	d, clicks := desktopWithFake(b)

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

func TestNotify_IgnoresAFailureToDialTheBus(t *testing.T) {
	t.Parallel()

	d := &notify.Desktop{
		Dial: func(context.Context) (notify.Bus, func(), error) { return nil, nil, errors.New("no session bus") },
	}

	d.Notify("title", "body", "conv-1") // must not panic or block
}

// TestNotify_DefaultsToDialingTheRealSessionBus confirms Notify falls
// back to dialSessionBus rather than panicking on a nil Dial func, and
// that it still does not block when there is nothing to connect to
// (every test runs with DBUS_SESSION_BUS_ADDRESS pointed nowhere, so
// this never reaches a real bus).
func TestNotify_DefaultsToDialingTheRealSessionBus(t *testing.T) {
	t.Parallel()

	d := &notify.Desktop{}
	d.Notify("title", "body", "conv-1") // must not panic or block
}

func TestNotify_IgnoresAFailedShowCall(t *testing.T) {
	t.Parallel()

	b := &fakeBus{showErr: errors.New("service refused the call")}
	d, clicks := desktopWithFake(b)

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

func TestNotify_IgnoresAFailedWaitCall(t *testing.T) {
	t.Parallel()

	b := &fakeBus{waitErr: errors.New("lost the connection"), signals: make(chan string, 1)}
	d, clicks := desktopWithFake(b)

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

// blockingBus is a Bus whose Show always succeeds and whose Wait never
// returns on its own, so a notification shown through it holds its
// concurrency slot until ctx ends - the same as a real notification
// nothing ever closes, up to notifyTimeout.
type blockingBus struct {
	mu    sync.Mutex
	shown []string
}

func (b *blockingBus) Show(_, title, _ string, _ []string, _ map[string]dbus.Variant) (uint32, error) {
	b.mu.Lock()
	b.shown = append(b.shown, title)
	b.mu.Unlock()

	return 1, nil
}

func (b *blockingBus) Wait(ctx context.Context, _ uint32) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (b *blockingBus) titles() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]string(nil), b.shown...)
}

// TestNotify_DropsBeyondTheConcurrencyCap confirms a burst of
// notifications - WhatsApp or Telegram delivering a backlog at once,
// say - never opens more than MaxConcurrentNotifications goroutines
// and bus connections: acquiring a slot happens synchronously inside
// Notify, so every call beyond the cap is dropped at once rather than
// racing the goroutines it would otherwise spawn.
func TestNotify_DropsBeyondTheConcurrencyCap(t *testing.T) {
	t.Parallel()

	b := &blockingBus{}
	d := &notify.Desktop{Dial: func(context.Context) (notify.Bus, func(), error) { return b, func() {}, nil }}

	for i := 0; i < notify.MaxConcurrentNotifications+3; i++ {
		d.Notify("title", "body", "conv-1")
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(b.titles()) < notify.MaxConcurrentNotifications && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := len(b.titles()); got != notify.MaxConcurrentNotifications {
		t.Errorf("Show was called %d times, want exactly MaxConcurrentNotifications (%d)", got, notify.MaxConcurrentNotifications)
	}
}

// TestNotify_ReleasesItsSlotOnceANotificationEnds confirms a slot a
// notification held is free again once its own run ends - here,
// because nothing ever closes it and notifyTimeout elapses - so a
// notification dropped while the cap was full can still show once
// room frees up, rather than being dropped forever.
func TestNotify_ReleasesItsSlotOnceANotificationEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &blockingBus{}
		d := &notify.Desktop{Dial: func(context.Context) (notify.Bus, func(), error) { return b, func() {}, nil }}

		for i := 0; i < notify.MaxConcurrentNotifications; i++ {
			d.Notify("filling", "body", "conv-1")
		}
		synctest.Wait()

		d.Notify("dropped while full", "body", "conv-1")
		synctest.Wait()

		for _, title := range b.titles() {
			if title == "dropped while full" {
				t.Fatal("Show was called while every slot was already in use")
			}
		}

		time.Sleep(6 * time.Minute) // past notifyTimeout: every blocked Wait gives up and releases its slot
		synctest.Wait()

		d.Notify("fits now", "body", "conv-1")
		synctest.Wait()

		found := false
		for _, title := range b.titles() {
			found = found || title == "fits now"
		}
		if !found {
			t.Error("Show was never called for a notification sent once a slot had freed up")
		}

		// Let this last notification's own goroutine finish too, so none
		// are left durably blocked when the bubble ends.
		time.Sleep(6 * time.Minute)
		synctest.Wait()
	})
}

// TestNotify_TruncatesAnOverlongTitleAndBodyOnARuneBoundary confirms a
// sender-controlled title or body far longer than any real one could
// usefully be is cut down before it ever reaches Show, on a rune
// boundary so a multi-byte character at the cut point is never split
// into invalid UTF-8.
func TestNotify_TruncatesAnOverlongTitleAndBodyOnARuneBoundary(t *testing.T) {
	t.Parallel()

	b := closedWithoutAction()
	d, clicks := desktopWithFake(b)

	longTitle := strings.Repeat("a", 500)
	longBody := strings.Repeat("🎉", 3000) // a multi-byte rune throughout, to prove the cut never splits one
	d.Notify(longTitle, longBody, "conv-1")
	assertNoClick(t, clicks)

	_, title, body, _, _ := b.args()
	if n := len([]rune(title)); n > 200 {
		t.Errorf("title has %d runes, want at most 200", n)
	}
	if n := len([]rune(body)); n > 2000 {
		t.Errorf("body has %d runes, want at most 2000", n)
	}
	if !utf8.ValidString(title) || !utf8.ValidString(body) {
		t.Error("truncated title or body is not valid UTF-8: the cut split a multi-byte rune")
	}
}

// TestServiceReachable_ReturnsFalseWithNoSessionBusConfigured confirms
// ServiceReachable fails closed, promptly, when there is nothing to
// connect to - the situation every test run is in, and many desktops
// without a notification daemon running too.
func TestServiceReachable_ReturnsFalseWithNoSessionBusConfigured(t *testing.T) {
	// Not parallel: t.Setenv forbids it.
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent")

	start := time.Now()
	if notify.ServiceReachable(t.Context()) {
		t.Error("ServiceReachable = true with no session bus configured, want false")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("ServiceReachable took %s to fail, want a prompt result", elapsed)
	}
}

// TestNotify_NeverSpawnsAnExternalProcess confirms notify.go has no
// os/exec import at all, so no code path in this package can ever put a
// notification's title, body or conversation id into another process's
// command line, readable from /proc by any other local user.
func TestNotify_NeverSpawnsAnExternalProcess(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "notify.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse notify.go: %v", err)
	}

	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "os/exec" {
			t.Fatal("notify.go imports os/exec: a notification could leak its content into a process's argv")
		}
	}
}
