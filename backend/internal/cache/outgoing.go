package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Outgoing stores the files a user attaches to outgoing messages, named
// by the message's id, so a retry can resend one even after the user
// moves, renames or deletes the original. It keeps everything it is
// given, unlike Cache, which only remembers the most recently used
// files: an attachment must still be here on a retry no matter how long
// that takes.
type Outgoing struct {
	dir string
}

// NewOutgoing returns an outgoing media area rooted at dir.
func NewOutgoing(dir string) *Outgoing {
	return &Outgoing{dir: dir}
}

// Store copies the bytes of r into the outgoing media area under id and
// fileName, readable only by the user, and returns the new path. A
// failed or interrupted copy leaves nothing behind.
func (o *Outgoing) Store(ctx context.Context, id, fileName string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(o.dir, 0o700); err != nil {
		return "", fmt.Errorf("cache: outgoing: %w", err)
	}

	dst := o.Path(id, fileName)
	if err := copyToFile(dst, r); err != nil {
		return "", fmt.Errorf("cache: outgoing: %w", err)
	}

	return dst, nil
}

// Path is where an attachment named fileName for message id is stored,
// found again without re-copying it, such as for a retry.
func (o *Outgoing) Path(id, fileName string) string {
	return filepath.Join(o.dir, id+"-"+fileName)
}

// copyToFile writes r to dst through a temporary file, so a failed or
// interrupted copy never leaves a partial file at dst, readable only by
// the user.
func copyToFile(dst string, r io.Reader) error {
	part := dst + partSuffix
	out, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, r); err != nil {
		return errors.Join(err, out.Close(), removeIfPresent(part))
	}

	if err := out.Close(); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	if err := os.Rename(part, dst); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	return nil
}
