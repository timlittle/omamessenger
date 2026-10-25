package cache

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Sounds stores a validated copy of every custom notification sound this
// helper plays, one file per key, so a sound keeps working even after the
// user moves, renames or deletes the file they originally picked. A key
// names a conversation or a service's own default and always overwrites
// its own file in place (Store), so callers never need to remove an old
// copy themselves before replacing it.
type Sounds struct {
	dir string
}

// NewSounds returns a notification sound store rooted at dir, created on
// first use.
func NewSounds(dir string) *Sounds {
	return &Sounds{dir: dir}
}

// Store copies the bytes of r into the sound store under key, readable
// only by the user, and returns the new path. A failed or interrupted
// copy leaves nothing behind. The caller is responsible for validating
// the copy (see audiosafe.Validate) before ever handing its path to a
// player; Store itself only copies bytes.
func (s *Sounds) Store(ctx context.Context, key string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("cache: sounds: %w", err)
	}

	dst := s.Path(key)
	if err := copyToFile(dst, r); err != nil {
		return "", fmt.Errorf("cache: sounds: %w", err)
	}

	return dst, nil
}

// Path is where key's sound is stored, found again without re-copying
// it. key is always one of this helper's own fixed strings (a
// conversation id it generated, or a fixed per-service name), never a
// sender's or a user's own text, so this needs no traversal guard the
// way Outgoing.Path's file name does.
func (s *Sounds) Path(key string) string {
	return filepath.Join(s.dir, key+".sound")
}

// Remove deletes key's stored sound. Removing one already gone, or one
// never stored, is not an error.
func (s *Sounds) Remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := removeIfPresent(s.Path(key)); err != nil {
		return fmt.Errorf("cache: sounds: remove: %w", err)
	}

	return nil
}
