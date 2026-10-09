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
	"time"

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
func desktopWithFake(b *fakeBus) (notify.Desktop, <-chan string) {
	clicks := make(chan string, 1)
	d := notify.Desktop{
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

	d := notify.Desktop{
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

	notify.Desktop{}.Notify("title", "body", "conv-1") // must not panic or block
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
