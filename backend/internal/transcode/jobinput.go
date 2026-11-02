package transcode

// jobinput.go gives the sandboxed ffmpeg a copy of the caller's own
// input file, named sandbox.InputBaseName, inside the job directory
// run already makes for every job - never the caller's own path,
// which sandbox.Spec.validate now refuses outright. That path can
// carry an outgoing attachment's own file name or a user-picked
// notification sound, and the literal string given as Spec.Input
// reaches bwrap's own argv, readable by any local user through
// /proc for the life of the job (see sandbox's own package doc
// comment).

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// maxJobInputCopyBytes bounds placeJobInput's own fallback copy,
// used only once linking the caller's file into the job directory
// has already failed (ordinarily because it is on a different
// filesystem): generous for every kind this package ever
// transcodes - a voice note, a reaction sound, a received video, a
// user-picked notification sound - while still giving the copy its
// own named cap, checked before a single byte is read, rather than
// trusting whatever size the caller's own file happens to be.
const maxJobInputCopyBytes = 512 << 20

// placeJobInput makes path - the caller's own input file - available
// inside jobDir at sandbox.InputBaseName, the only base name
// sandbox.Spec.validate accepts for Spec.Input, and returns that new
// path. A hard link costs no extra space and no copy; placeJobInput
// falls back to an ordinary, size-capped copy only once linking has
// already failed, which happens whenever path and jobDir are not on
// the same filesystem.
func placeJobInput(path, jobDir string) (string, error) {
	dst := filepath.Join(jobDir, sandbox.InputBaseName)

	if err := os.Link(path, dst); err == nil {
		return dst, nil
	}
	if err := copyJobInput(path, dst); err != nil {
		return "", fmt.Errorf("transcode: job input: %w", err)
	}
	return dst, nil
}

// copyJobInput copies src to dst, refusing a src over
// maxJobInputCopyBytes before it reads a single byte of it.
func copyJobInput(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("%w: statting input: %w", ErrFailed, err)
	}
	if info.Size() > maxJobInputCopyBytes {
		return fmt.Errorf("%w: input is %d bytes, more than %d allowed", ErrFailed, info.Size(), maxJobInputCopyBytes)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%w: opening input: %w", ErrFailed, err)
	}
	defer func() {
		_ = in.Close() // read-only handle: nothing left to report once the copy below has already returned its own error
	}()

	return writeJobInputCopy(dst, in)
}

// writeJobInputCopy creates dst and copies all of in into it, leaving
// dst's own Close - and any error it reports - to this call, so
// copyJobInput's own body stays within this project's line limit.
func writeJobInputCopy(dst string, in io.Reader) error {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%w: creating job input copy: %w", ErrFailed, err)
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("%w: copying job input: %w", ErrFailed, copyErr)
	case closeErr != nil:
		return fmt.Errorf("%w: closing job input copy: %w", ErrFailed, closeErr)
	default:
		return nil
	}
}
