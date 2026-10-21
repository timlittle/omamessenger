package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FetchNamed is Fetch's counterpart for a file whose correct name can
// only be known once its bytes exist: nameFor reads what fill just
// wrote and returns the base name to keep it under - typically id plus
// an extension chosen from the file's own sniffed content, never one
// trusted upfront from whoever sent it (see app.mediaFileName and
// docs/decisions.md). id alone locks and locates an existing cached
// file, under whatever name nameFor gave it last time, so a second
// FetchNamed call for the same id never re-downloads, or re-asks
// nameFor, once the first has finished.
func (c *Cache) FetchNamed(ctx context.Context, id string, fill func(ctx context.Context, path string) error, nameFor func(path string) (string, error)) (string, error) {
	if !validName(id) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, id)
	}

	defer c.lockName(id)()

	if path, ok, err := c.findByID(id); err != nil || ok {
		if err != nil {
			return "", err
		}

		now := time.Now()

		return path, os.Chtimes(path, now, now)
	}

	name, err := c.fillNamed(ctx, id, fill, nameFor)
	if err != nil {
		return "", err
	}

	path := filepath.Join(c.dir, name)
	if err := c.rejectIfOversized(path); err != nil {
		return "", err
	}

	return path, c.prune(name)
}

// fillNamed writes a fresh file for id through fill, into a path only
// this id's own fill ever uses, then asks nameFor what to keep it as
// and renames it into place under that name, the same way fill does
// for Fetch's own upfront name.
func (c *Cache) fillNamed(ctx context.Context, id string, fill func(ctx context.Context, path string) error, nameFor func(path string) (string, error)) (string, error) {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return "", fmt.Errorf("cache: %w", err)
	}

	part := filepath.Join(c.dir, id+partSuffix)
	if err := fill(ctx, part); err != nil {
		return "", errors.Join(err, removeIfPresent(part))
	}

	name, err := nameOrInvalid(part, nameFor)
	if err != nil {
		return "", err
	}

	if err := os.Chmod(part, 0o600); err != nil {
		return "", fmt.Errorf("cache: %w", errors.Join(err, removeIfPresent(part)))
	}

	if err := os.Rename(part, filepath.Join(c.dir, name)); err != nil {
		return "", fmt.Errorf("cache: %w", errors.Join(err, removeIfPresent(part)))
	}

	return name, nil
}

// nameOrInvalid runs nameFor and checks its result is a safe base name,
// cleaning up part on either failure so fillNamed's caller never has to.
func nameOrInvalid(part string, nameFor func(path string) (string, error)) (string, error) {
	name, err := nameFor(part)
	if err != nil {
		return "", errors.Join(fmt.Errorf("cache: %w", err), removeIfPresent(part))
	}
	if !validName(name) {
		return "", errors.Join(fmt.Errorf("cache: %w: %q", ErrInvalidName, name), removeIfPresent(part))
	}

	return name, nil
}

// findByID returns the path of a file already cached for id, under
// whatever name it was given, or ok=false when there is none yet. A
// match requires the character right after id, if any, to be a '.':
// id "m1" must never match a different message's "m12.jpg", only its
// own "m1" or "m1.<ext>".
func (c *Cache) findByID(id string) (string, bool, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("cache: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, id) || strings.HasSuffix(name, partSuffix) {
			continue
		}
		if len(name) > len(id) && name[len(id)] != '.' {
			continue
		}

		return filepath.Join(c.dir, name), true, nil
	}

	return "", false, nil
}

// validName reports whether name is safe to use as a cache file's own
// base name: not empty, not "." or "..", holding no directory
// separator, and not already marked as a file still being filled.
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && name == filepath.Base(name) && !strings.HasSuffix(name, partSuffix)
}
