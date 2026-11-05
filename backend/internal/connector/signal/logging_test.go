package signal

import (
	"bytes"
	"log"
	"testing"
)

// TestLogWriter_TrimsNewline checks that a logged line's trailing
// newline, which zerolog always writes, does not become a blank line
// in the standard logger's own output.
func TestLogWriter_TrimsNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := logWriter{log: log.New(&buf, "", 0)}

	n, err := w.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len("hello\n") {
		t.Errorf("Write() = %d, want %d", n, len("hello\n"))
	}
	if got := buf.String(); got != "hello\n" {
		t.Errorf("logged = %q, want %q", got, "hello\n")
	}
}

// TestLogWriter_NoTrailingNewline checks a line with no trailing
// newline is logged unchanged.
func TestLogWriter_NoTrailingNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := logWriter{log: log.New(&buf, "", 0)}

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := buf.String(); got != "hello\n" {
		t.Errorf("logged = %q, want %q", got, "hello\n")
	}
}

// TestSetUpLogging_DoesNotPanic checks that signalmeow's own logger
// can be pointed at a standard logger without reaching libsignal-ffi
// in a way that panics. signalmeow guards SetLogger to run only once
// per process, so this cannot assert anything about the level it
// installed without a real libsignal-ffi call, which risks double
// initialization across the whole test binary.
func TestSetUpLogging_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	SetUpLogging(log.New(&buf, "", 0))
}
