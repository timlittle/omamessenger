// Package clipboard reads the Wayland clipboard through wl-paste. It
// exists because Quickshell's QML has no way to read clipboard image
// data itself, only ask whether a paste happened; wl-paste is the normal
// command-line way into a Wayland compositor's clipboard.
package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// commandTimeout bounds how long wl-paste may run, so a stuck clipboard
// portal cannot block the helper forever.
const commandTimeout = 3 * time.Second

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

// run invokes wl-paste with args and returns its stdout, bounded by
// commandTimeout.
func (w Wayland) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := w.command(ctx, "wl-paste", args...)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// command builds the process through Command, or exec.CommandContext
// when none was set.
func (w Wayland) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if w.Command != nil {
		return w.Command(ctx, name, args...)
	}

	return exec.CommandContext(ctx, name, args...) //nolint:gosec // deliberate: name is always a literal this package's own callers pass, never untrusted input
}
