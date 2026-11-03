package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"syscall"
	"testing"

	"go.mau.fi/libsignal/logger"
)

// TestClaimProtocolStdout_RedirectsStrayWritesAwayFromTheOriginal
// proves the swap this package's own protocol depends on: once
// claimProtocolStdout has run, a write through the *os.File it
// returns still reaches the original stream, but neither fmt.Println
// (which always writes through the os.Stdout package variable) nor a
// direct write through the very fd number the protocol used to own
// reaches it any more - both land on the redirect target instead,
// exactly the shape go.mau.fi/libsignal's own default logger's stray
// fmt.Println must take.
func TestClaimProtocolStdout_RedirectsStrayWritesAwayFromTheOriginal(t *testing.T) {
	protocolRead, protocolWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (protocol side) = %v", err)
	}
	t.Cleanup(func() { _ = protocolRead.Close() })

	redirectRead, redirectWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (redirect side) = %v", err)
	}
	t.Cleanup(func() { _ = redirectRead.Close() })

	originalStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = originalStdout })

	protocol, err := claimProtocolStdout(int(protocolWrite.Fd()), redirectWrite)
	if err != nil {
		t.Fatalf("claimProtocolStdout() error = %v, want nil", err)
	}

	if _, err := protocol.WriteString("protocol line\n"); err != nil {
		t.Fatalf("write through the returned protocol file: %v", err)
	}
	if _, err := fmt.Println("leaked via fmt.Println"); err != nil {
		t.Fatalf("fmt.Println: %v", err)
	}
	if _, err := protocolWrite.WriteString("leaked via the stolen fd\n"); err != nil {
		t.Fatalf("write through the stolen fd: %v", err)
	}

	_ = protocol.Close()
	_ = protocolWrite.Close()
	_ = redirectWrite.Close()

	protocolGot, err := io.ReadAll(protocolRead)
	if err != nil {
		t.Fatalf("read protocol side: %v", err)
	}
	redirectGot, err := io.ReadAll(redirectRead)
	if err != nil {
		t.Fatalf("read redirect side: %v", err)
	}

	if got := string(protocolGot); got != "protocol line\n" {
		t.Errorf("protocol side received %q, want only the one write made through the returned file", got)
	}
	if got := string(redirectGot); !strings.Contains(got, "leaked via fmt.Println") || !strings.Contains(got, "leaked via the stolen fd") {
		t.Errorf("redirect side = %q, want both stray writes", got)
	}
	if strings.Contains(string(redirectGot), "protocol line") {
		t.Error("the protocol's own write reached the redirect side too, want it isolated")
	}
}

// TestClaimProtocolStdout_ReportsAnInvalidDescriptor covers the one
// error path: a file descriptor that cannot be duplicated at all.
func TestClaimProtocolStdout_ReportsAnInvalidDescriptor(t *testing.T) {
	if _, err := claimProtocolStdout(-1, os.Stderr); err == nil {
		t.Error("claimProtocolStdout(-1, ...) succeeded, want an error")
	}
}

// TestClaimProtocolStdout_DuplicatedDescriptorIsCloseOnExec confirms
// the descriptor claimProtocolStdout duplicates for the protocol
// writer's own exclusive use is marked close-on-exec: a security
// review found syscall.Dup's own result carries no such flag, unlike
// every descriptor the os package itself opens, so without this a
// child this helper starts later - wl-copy, wl-paste, or bwrap for the
// sandbox - would inherit a second, writable copy of the protocol
// stream.
func TestClaimProtocolStdout_DuplicatedDescriptorIsCloseOnExec(t *testing.T) {
	protocolRead, protocolWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (protocol side) = %v", err)
	}
	t.Cleanup(func() { _ = protocolRead.Close() })

	_, redirectWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (redirect side) = %v", err)
	}

	originalStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = originalStdout })

	protocol, err := claimProtocolStdout(int(protocolWrite.Fd()), redirectWrite)
	if err != nil {
		t.Fatalf("claimProtocolStdout() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = protocol.Close() })
	t.Cleanup(func() { _ = protocolWrite.Close() })
	t.Cleanup(func() { _ = redirectWrite.Close() })

	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, protocol.Fd(), uintptr(syscall.F_GETFD), 0)
	if errno != 0 {
		t.Fatalf("fcntl(F_GETFD) = %v", errno)
	}
	if flags&syscall.FD_CLOEXEC == 0 {
		t.Error("the duplicated protocol descriptor is not close-on-exec, so a child process would inherit a writable copy of it")
	}
}

// TestLibsignalLogger_DropsDebugAndInfo confirms the two noisiest
// levels never reach the log at all.
func TestLibsignalLogger_DropsDebugAndInfo(t *testing.T) {
	var buf bytes.Buffer
	l := libsignalLogger{log: log.New(&buf, "", 0)}

	l.Debug("Session.go:1", "a debug message")
	l.Info("Session.go:2", "an info message")

	if buf.Len() != 0 {
		t.Errorf("log = %q after Debug and Info, want empty", buf.String())
	}
}

// TestLibsignalLogger_WritesWarningAndError confirms both levels
// reach the log with their own caller and message text.
func TestLibsignalLogger_WritesWarningAndError(t *testing.T) {
	var buf bytes.Buffer
	l := libsignalLogger{log: log.New(&buf, "", 0)}

	l.Warning("SessionCipher.go:263", "a warning message")
	l.Error("SessionCipher.go:298", "an error message")

	got := buf.String()
	for _, want := range []string{"SessionCipher.go:263", "a warning message", "SessionCipher.go:298", "an error message"} {
		if !strings.Contains(got, want) {
			t.Errorf("log = %q, want it to contain %q", got, want)
		}
	}
}

// TestQuietLibsignalLogger_ConfiguresThePackageLogger confirms
// quietLibsignalLogger actually wires itself up as
// go.mau.fi/libsignal's own shared logger, rather than only building
// a value nothing ever installs.
func TestQuietLibsignalLogger_ConfiguresThePackageLogger(t *testing.T) {
	var buf bytes.Buffer
	quietLibsignalLogger(log.New(&buf, "", 0))

	logger.Warning("a warning from the package-level logger")

	if got := buf.String(); !strings.Contains(got, "a warning from the package-level logger") {
		t.Errorf("log = %q, want the package-level Warning call to reach it", got)
	}
}
