package transcode

// jobinput.go gives the sandboxed ffmpeg a copy of the caller's input
// file inside the job directory, never the caller's own path: that
// path would otherwise be readable by any local user through /proc
// for the life of the job.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// maxJobInputCopyBytes bounds placeJobInput's fallback copy, used
// when a hard link fails because the files are on different
// filesystems, so the copy never trusts an unbounded file size.
const maxJobInputCopyBytes = 512 << 20

// placeJobInput makes path available inside jobDir at
// sandbox.InputBaseName and returns the new path, preferring a hard
// link and falling back to a size-capped copy across filesystems.
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
