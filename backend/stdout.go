package main

// stdout.go makes the real stdout file descriptor physically
// unreachable by anything in this process but the protocol writer
// server.New wires up. protocol.md reserves stdout for JSON-RPC
// traffic alone: a dependency that prints to it by mistake - or by
// its own default, the way go.mau.fi/libsignal's logger falls back
// to fmt.Println when nothing configures it - would inject a line
// the UI's own line-splitting parser cannot make sense of into the
// one stream it is reading as protocol. See docs/decisions.md.

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"go.mau.fi/libsignal/logger"
)

// claimProtocolStdout duplicates the file descriptor fd currently
// names for the protocol writer's own exclusive use, then points fd
// itself, and the os.Stdout package variable every dependency that
// writes through it shares, at redirectTo instead. It must run
// before any dependency has a chance to write to stdout - first thing
// in exitCode, before store.Open, the connectors or anything else
// initializes - or a write that already happened before the swap is
// not undone by it. It returns the duplicated *os.File callers must
// use in place of os.Stdout for the protocol itself, or an error if
// either syscall failed, which the caller should treat as fatal:
// continuing on would risk a stray write reaching the real stdout
// after all.
func claimProtocolStdout(fd int, redirectTo *os.File) (*os.File, error) {
	dupFD, err := syscall.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("claim protocol stdout: duplicate fd %d: %w", fd, err)
	}
	protocol := os.NewFile(uintptr(dupFD), "protocol-stdout")

	if err := syscall.Dup2(int(redirectTo.Fd()), fd); err != nil {
		return nil, fmt.Errorf("claim protocol stdout: redirect fd %d: %w", fd, err)
	}

	os.Stdout = redirectTo
	return protocol, nil
}

// quietLibsignalLogger configures go.mau.fi/libsignal's own
// package-level logger - a global the Signal protocol code this
// helper's WhatsApp connector depends on reaches for through, with no
// way to inject one through app.Deps - to write to logger instead of
// falling back to its own built-in default, which prints every
// warning and error straight to stdout (see claimProtocolStdout's own
// comment). Called once, from main, before anything that could touch
// the Signal protocol runs: go.mau.fi/libsignal's own lazy
// default-logger initialization has no synchronization, so setting a
// logger up front also closes a data race two goroutines signing with
// a fresh device at once would otherwise hit.
func quietLibsignalLogger(out *log.Logger) {
	var loggable logger.Loggable = libsignalLogger{log: out}
	logger.Setup(&loggable)
}

// libsignalLogger adapts logger.Loggable to this helper's own
// stderr-only logger: debug and info messages are dropped outright,
// since nothing at either level is worth the noise, and warning and
// error messages are written through, a crypto or protobuf error's
// own text and the file and line it came from - never message
// content, a session key or a JID (see privacy.md) - the same
// category-only discipline every other log call in this helper
// already follows.
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
