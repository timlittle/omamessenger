package main

// stdout.go makes the real stdout file descriptor reachable only by
// the protocol writer, so a dependency cannot accidentally print a
// stray line into the JSON-RPC stream (see docs/decisions.md).

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"go.mau.fi/libsignal/logger"
)

// claimProtocolStdout duplicates fd for the protocol writer's
// exclusive use, then redirects fd and os.Stdout to redirectTo. It
// must run before anything else in exitCode could write to stdout.
// The duplicate is marked close-on-exec so a later child process
// (wl-copy, wl-paste, bwrap) never inherits a writable copy of the
// protocol stream.
func claimProtocolStdout(fd int, redirectTo *os.File) (*os.File, error) {
	dupFD, err := syscall.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("claim protocol stdout: duplicate fd %d: %w", fd, err)
	}
	syscall.CloseOnExec(dupFD)
	protocol := os.NewFile(uintptr(dupFD), "protocol-stdout")

	if err := syscall.Dup3(int(redirectTo.Fd()), fd, 0); err != nil {
		return nil, fmt.Errorf("claim protocol stdout: redirect fd %d: %w", fd, err)
	}

	os.Stdout = redirectTo
	return protocol, nil
}

// quietLibsignalLogger points go.mau.fi/libsignal's package-level
// logger at out instead of its default, which prints straight to
// stdout. Call once from main before any Signal protocol code runs,
// since its lazy default-logger init is also otherwise unsynchronized.
func quietLibsignalLogger(out *log.Logger) {
	var loggable logger.Loggable = libsignalLogger{log: out}
	logger.Setup(&loggable)
}

// libsignalLogger adapts logger.Loggable to this helper's own
// stderr-only logger, dropping debug and info, and logging only the
// caller and message text for warning and error (see privacy.md).
type libsignalLogger struct {
	log *log.Logger
}

// Debug drops every debug-level message outright.
func (libsignalLogger) Debug(caller, message string) {}

// Info drops every info-level message outright.
func (libsignalLogger) Info(caller, message string) {}

// Warning writes a warning-level message to l's own log.
func (l libsignalLogger) Warning(caller, message string) {
	l.log.Printf("libsignal warning (%s): %s", caller, message)
}

// Error writes an error-level message to l's own log.
func (l libsignalLogger) Error(caller, message string) {
	l.log.Printf("libsignal error (%s): %s", caller, message)
}

// Configure is a no-op: libsignalLogger logs by level alone and does
// not support the default logger's own per-file namespace filtering.
func (libsignalLogger) Configure(settings string) {}
