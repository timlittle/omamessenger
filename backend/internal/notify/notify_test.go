package notify

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDesktopUsesNotifySendArgv(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	stubPath := filepath.Join(dir, "notify-send")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NOTIFY_TEST_ARGS\"\n"
	if err := os.WriteFile(stubPath, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("NOTIFY_TEST_ARGS", argsPath)

	(Desktop{}).Notify("Alex · Work", "New message")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(argsPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("notify-send stub did not record arguments: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{"--app-name=OmaMessenger", "--category=im.received", "--", "Alex · Work", "New message"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("notify-send argv = %#v, want %#v", got, want)
	}
}

func TestDesktopIgnoresMissingNotifySend(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	(Desktop{}).Notify("title", "body")
}
