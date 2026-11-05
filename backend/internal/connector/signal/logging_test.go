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

// TestSetUpLogging_DoesNotPanic checks that pointing signalmeow's
// logger at a standard logger does not panic.
func TestSetUpLogging_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	SetUpLogging(log.New(&buf, "", 0))
}
