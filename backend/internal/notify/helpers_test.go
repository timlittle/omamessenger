// Package notify's own test helper: a private, throwaway session bus so
// dialSessionBus and ServiceReachable can be exercised against a real
// D-Bus connection without ever reaching the developer's own session
// bus. It is shared by notify_internal_test.go's white-box tests.
package notify

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// startTestBus starts a throwaway dbus-daemon for the calling test,
// points DBUS_SESSION_BUS_ADDRESS at it, and kills it in Cleanup. It
// skips the test, rather than failing it, on a machine with no
// dbus-daemon installed.
func startTestBus(t *testing.T) {
	t.Helper()

	path, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed; skipping the live session bus test")
	}

	cmd := exec.CommandContext(t.Context(), path, "--session", "--print-address", "--nofork")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("dbus-daemon stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // the test is over either way; nothing left to report
		_ = cmd.Wait()         // reap it so it does not linger as a zombie
	})

	address, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read dbus-daemon address: %v", err)
	}

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(address))
}
