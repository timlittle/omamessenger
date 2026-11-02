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

// soundExtension is the only extension Store itself ever writes: a
// name no build of this helper, released or not, ever used before
// this one (see Sweep and docs/decisions.md). Resolving a sound by
// this exact extension, rather than trusting whatever already sits
// under a key's name, is what keeps a stray file left by an older
// build from ever being served.
const soundExtension = ".wav"

// Sounds stores a validated copy of every custom notification sound this
// helper plays, one file per key, so a sound keeps working even after the
// user moves, renames or deletes the file they originally picked. A key
// names a conversation or a service's own default and always overwrites
// its own file in place (Store), so callers never need to remove an old
// copy themselves before replacing it.
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

// lockKey reserves key's own lock, waiting for any other call already
// holding it, and returns the func that releases it: two Store calls
// for the same key must never run at once, or they could both be
// mid-copy to the exact same ".part" temporary file at once (see
// copyToFile and docs/decisions.md). Sounds has at most a handful of
// keys - one per conversation with its own custom sound, plus one per
// service - all chosen by the user, never a sender, so this map
// simply grows with that small, bounded set rather than needing
// Cache's own reference-counted cleanup (see Cache.lockName).
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
// copy leaves nothing behind. The caller is responsible for transcoding
// r into a format the player can trust before it ever reaches here (see
// app.storeAndTranscodeSound and docs/decisions.md); Store itself only
// copies bytes. Two calls for the same key never run at once (see
// lockKey).
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

// Path is where key's sound is stored, found again without re-copying
// it. key is always one of this helper's own fixed strings (a
// conversation id it generated, or a fixed per-service name), never a
// sender's or a user's own text, so this needs no traversal guard the
// way Outgoing.Path's file name does.
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

// Sweep removes every file in the sound store that is not named
// <key>.wav: anything else cannot have come from this helper's own
// Store, however it got there (see docs/decisions.md). Called once at
// startup, before anything can resolve a sound to play, so a file an
// older build once left behind is gone before the first request that
// could have served it.
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
