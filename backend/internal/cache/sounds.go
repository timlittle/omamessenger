package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// soundExtension is the only extension Store writes, so Sweep can tell
// its files from anything left by an older build.
const soundExtension = ".wav"

// Sounds stores a validated copy of every custom notification sound this
// helper plays, one file per key (a conversation or a service's own
// default), so a sound keeps working after the user moves or deletes the
// original file.
type Sounds struct {
	dir string

	keysMu sync.Mutex
	keys   map[string]*sync.Mutex
}

// NewSounds returns a notification sound store rooted at dir, created on
// first use.
func NewSounds(dir string) *Sounds {
	return &Sounds{dir: dir}
}

// lockKey reserves key's own lock and returns the func that releases
// it, so two Store calls for the same key never collide mid-copy to
// the same temporary file. The key set is small and user-chosen, so
// this map just grows rather than needing reference-counted cleanup.
func (s *Sounds) lockKey(key string) func() {
	s.keysMu.Lock()
	if s.keys == nil {
		s.keys = map[string]*sync.Mutex{}
	}
	l, ok := s.keys[key]
	if !ok {
		l = &sync.Mutex{}
		s.keys[key] = l
	}
	s.keysMu.Unlock()

	l.Lock()

	return l.Unlock
}

// Store copies the bytes of r into the sound store under key, readable
// only by the user, and returns the new path. A failed or interrupted
// copy leaves nothing behind; the caller must transcode r into a
// trusted format before calling this (app.storeAndTranscodeSound).
func (s *Sounds) Store(ctx context.Context, key string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	unlock := s.lockKey(key)
	defer unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("cache: sounds: %w", err)
	}

	dst := s.Path(key)
	if err := copyToFile(dst, r); err != nil {
		return "", fmt.Errorf("cache: sounds: %w", err)
	}

	return dst, nil
}

// Path is where key's sound is stored. key is always one of this
// helper's own fixed strings, never a sender's or user's text, so this
// needs no traversal guard.
func (s *Sounds) Path(key string) string {
	return filepath.Join(s.dir, key+soundExtension)
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

// Sweep removes every file in the sound store not named <key>.wav, so
// a file left by an older build is gone before anything can resolve
// it. Called once at startup.
func (s *Sounds) Sweep(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("cache: sounds: sweep: %w", err)
	}

	var errs []error
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), soundExtension) {
			continue
		}

		errs = append(errs, removeIfPresent(filepath.Join(s.dir, e.Name())))
	}

	return errors.Join(errs...)
}
