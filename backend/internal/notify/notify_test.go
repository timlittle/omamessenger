// Desktop's Command field replaces notify-send itself, so every test here
// runs a stub process instead of the real one and can run in parallel.
package notify_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/notify"
)

// stub writes a notify-send look-alike to dir that records its arguments
// into argsPath and, if action is non-empty, prints it to stdout the way
// notify-send prints the name of the action the user chose.
func stub(t *testing.T, dir, argsPath, action string) string {
	t.Helper()

	path := filepath.Join(dir, "notify-send")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\n"
	if action != "" {
		script += "printf '%s\\n' \"" + action + "\"\n"
	}

	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	return path
}

// desktopWithStub returns a Desktop whose Command runs the stub at path
// regardless of the name Notify asked for, and a channel fed by click.
func desktopWithStub(path string) (notify.Desktop, <-chan string) {
	clicks := make(chan string, 1)
	d := notify.Desktop{
		Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, path, args...)
		},
		Click: func(conversationID string) { clicks <- conversationID },
	}

	return d, clicks
}

// waitForClick returns the conversation id Click was called with, or fails
// the test if the stub process never reports one.
func waitForClick(t *testing.T, clicks <-chan string) string {
	t.Helper()

	select {
	case id := <-clicks:
		return id
	case <-time.After(2 * time.Second):
		t.Fatal("click was never reported")
		return ""
	}
}

// assertNoClick fails the test if clicks receives anything within a
// window well beyond how long the local stub process needs to exit.
func assertNoClick(t *testing.T, clicks <-chan string) {
	t.Helper()

	select {
	case id := <-clicks:
		t.Fatalf("unexpected click for conversation %q", id)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNotify_RunsNotifySendWithTitleBodyAndDefaultAction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	d, clicks := desktopWithStub(stub(t, dir, argsPath, ""))

	d.Notify("Alex · Work", "New message", "conv-1")
	assertNoClick(t, clicks)

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("stub did not run: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{
		"--app-name=OmaMessenger", "--category=im.received", "--action=default=Open",
		"--", "Alex · Work", "New message",
	}
	if !slices.Equal(got, want) {
		t.Errorf("notify-send arguments = %q, want %q", got, want)
	}
}

func TestNotify_ReportsAClickWithTheConversationID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	d, clicks := desktopWithStub(stub(t, dir, filepath.Join(dir, "args"), "default"))

	d.Notify("title", "body", "conv-42")

	if got := waitForClick(t, clicks); got != "conv-42" {
		t.Errorf("click reported conversation %q, want %q", got, "conv-42")
	}
}

func TestNotify_IgnoresANotificationClosedWithoutAClick(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	d, clicks := desktopWithStub(stub(t, dir, filepath.Join(dir, "args"), ""))

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

func TestNotify_IgnoresAnUnrelatedLineOfOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	d, clicks := desktopWithStub(stub(t, dir, filepath.Join(dir, "args"), "0"))

	d.Notify("title", "body", "conv-1")

	assertNoClick(t, clicks)
}

func TestNotify_IgnoresMissingNotifySend(t *testing.T) {
	t.Parallel()

	d := notify.Desktop{
		Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, filepath.Join(t.TempDir(), "no-such-binary"), args...)
		},
	}

	d.Notify("title", "body", "conv-1") // must not panic or block
}

func TestNotify_DefaultsToTheRealNotifySendCommand(t *testing.T) {
	// Not parallel: t.Setenv forbids it.
	// No Command set: Notify must fall back to exec.CommandContext rather
	// than panic on a nil func. Point PATH at a stub so nothing real runs.
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	stub(t, dir, argsPath, "")
	t.Setenv("PATH", dir)

	notify.Desktop{}.Notify("title", "body", "conv-1")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.ReadFile(argsPath); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("stub notify-send on PATH was never run")
}

// Desktop must still see the "default" action when notify-send's last
// write has no trailing newline; bufio.Scanner yields that final line too.
func TestNotify_ReadsAFinalLineWithoutATrailingNewline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "notify-send")
	script := "#!/bin/sh\nprintf 'default'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	d, clicks := desktopWithStub(path)
	d.Notify("title", "body", "conv-9")

	if got := waitForClick(t, clicks); got != "conv-9" {
		t.Errorf("click reported conversation %q, want %q", got, "conv-9")
	}
}
