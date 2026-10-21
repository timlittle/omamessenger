// Package clipboard reads the Wayland clipboard through wl-paste. It
// exists because Quickshell's QML has no way to read clipboard image
// data itself, only ask whether a paste happened; wl-paste is the normal
// command-line way into a Wayland compositor's clipboard.
package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// commandTimeout bounds how long wl-paste may run, so a stuck clipboard
// portal cannot block the helper forever.
const commandTimeout = 3 * time.Second

// writeTimeout bounds how long Write may take handing data to wl-copy,
// which must read all of it before it can fork into the background and
// start serving paste requests; longer than commandTimeout since a
// copied photo or video can be considerably larger than anything this
// helper ever pastes in.
const writeTimeout = 10 * time.Second

// MaxPasteLen bounds how many bytes Read accepts from wl-paste: the
// same cap as the full-size image profile a pasted photo is sanitized
// against (see app/paste.go and imagesafe.FullImage), large enough for
// any image this UI would otherwise accept, small enough that a stuck
// or hostile clipboard source cannot grow this process's memory
// without bound.
const MaxPasteLen = 32 << 20

// errPasteTooLarge reports that wl-paste's own output was larger than
// MaxPasteLen: the clipboard belongs to whichever other program last
// put something on it, so its declared or actual size is never
// trusted any more than a message attachment's.
var errPasteTooLarge = errors.New("clipboard: paste exceeds the size limit")

// Wayland reads the clipboard with wl-paste.
type Wayland struct {
	// Command builds the process that runs one wl-paste call. Nil uses
	// exec.CommandContext; tests replace it with a stub so nothing real
	// runs.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// Types lists the MIME types wl-paste says the clipboard currently
// offers, or none when it is empty.
func (w Wayland) Types(ctx context.Context) ([]string, error) {
	out, err := w.run(ctx, "--list-types")
	if err != nil {
		return nil, fmt.Errorf("clipboard: list types: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	return strings.Split(trimmed, "\n"), nil
}

// Read writes the clipboard's data of mimeType to w.
func (w Wayland) Read(ctx context.Context, mimeType string, dst io.Writer) error {
	out, err := w.run(ctx, "--type", mimeType)
	if err != nil {
		return fmt.Errorf("clipboard: read: %w", err)
	}

	_, err = dst.Write(out)

	return err
}

// Write puts data on the clipboard as mimeType, through wl-copy,
// bounded by writeTimeout: the counterpart to Read, for a photo, video
// or other media this helper has already downloaded or is about to
// send, so QML never has to shell out to copy anything itself.
func (w Wayland) Write(ctx context.Context, mimeType string, data io.Reader) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	cmd := w.command(ctx, "wl-copy", "--type", mimeType)
	cmd.Stdin = data

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard: write: %w", err)
	}

	return nil
}

// run invokes wl-paste with args and returns its stdout, bounded by
// commandTimeout and MaxPasteLen.
func (w Wayland) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := w.command(ctx, "wl-paste", args...)

	out := &boundedWriter{max: MaxPasteLen}
	cmd.Stdout = out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return out.buf.Bytes(), nil
}

// boundedWriter collects what is written to it the same way
// bytes.Buffer does, but fails once more than max bytes have arrived,
// so wl-paste's own output can never grow this process's memory past
// the clipboard's declared cap, however large the clipboard source
// claims to be.
type boundedWriter struct {
	buf bytes.Buffer
	max int
}

// Write appends p, refusing once the total written would exceed max bytes.
func (w *boundedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, errPasteTooLarge
	}

	return w.buf.Write(p)
}

// command builds the process through Command, or exec.CommandContext
// when none was set.
func (w Wayland) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if w.Command != nil {
		return w.Command(ctx, name, args...)
	}

	return exec.CommandContext(ctx, name, args...) //nolint:gosec,forbidigo // deliberate: name is always a literal this package's own callers pass, never untrusted input; this is the one file allowed to launch a subprocess
}
