// These tests replace notify-send through PATH, so they cannot run in
// parallel with each other.
package notify_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/notify"
)

func TestNotify_RunsNotifySend(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NOTIFY_TEST_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	t.Setenv("NOTIFY_TEST_ARGS", argsPath)

	notify.Desktop{}.Notify("Alex · Work", "New message")

	data := waitForFile(t, argsPath, "New message\n")
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{"--app-name=OmaMessenger", "--category=im.received", "--", "Alex · Work", "New message"}
	if !slices.Equal(got, want) {
		t.Errorf("notify-send arguments = %q, want %q", got, want)
	}
}

func TestNotify_IgnoresMissingNotifySend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	notify.Desktop{}.Notify("title", "body") // must not panic or block
}

// waitForFile returns the file's contents once they end with suffix.
// notify-send runs as a separate process, so there is nothing to
// synchronise on but the file.
func waitForFile(t *testing.T, path, suffix string) []byte {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(data), suffix) {
			return data
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("notify-send was not run")

	return nil
}
