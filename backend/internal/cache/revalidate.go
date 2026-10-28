package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RevalidateCheck decides what Revalidate should do with one file
// already in a media area: true, with a nil error, to leave it in
// place, unchanged or already rewritten by check itself. Anything
// else - false, or an error because path could not even be read -
// has Revalidate remove it: a file this sweep could not even look at
// is never kept just because it failed to be checked, any more than
// one it could check and rejected. Revalidate reports the error but
// still moves on to the rest of the sweep, rather than stopping over
// one bad file.
type RevalidateCheck func(path string) (keep bool, err error)

// Revalidate calls check for every finished file already in the
// cache - never one still being filled (see partSuffix) or its own
// format marker - removing any it reports keep=false for. It is for
// a one-time sweep of files a now-older version of this helper
// wrote, never for ordinary serving, where Fetch and FetchNamed
// already own a file's whole lifetime; see docs/decisions.md. A
// directory not created yet simply holds nothing to revalidate, not
// an error.
func (c *Cache) Revalidate(check RevalidateCheck) error {
	return revalidateDir(c.dir, check)
}

// Revalidate is Cache's own Revalidate, over the outgoing attachment
// area's directory instead.
func (o *Outgoing) Revalidate(check RevalidateCheck) error {
	return revalidateDir(o.dir, check)
}

// revalidateDir is Revalidate's own implementation, shared by Cache
// and Outgoing since both keep a flat directory of files named by a
// media area's own convention and nothing more.
func revalidateDir(dir string, check RevalidateCheck) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("cache: revalidate: %w", err)
	}

	var errs []error
	for _, e := range entries {
		if err := revalidateEntry(dir, e, check); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// revalidateEntry offers one directory entry to check, skipping a
// fill still in progress and the area's own format marker, and
// removes it unless check reports keep=true with no error: a check
// that failed to even read the file is never treated as a reason to
// leave it alone (see RevalidateCheck).
func revalidateEntry(dir string, e os.DirEntry, check RevalidateCheck) error {
	name := e.Name()
	if !e.Type().IsRegular() || strings.HasSuffix(name, partSuffix) || name == formatMarkerName {
		return nil
	}

	path := filepath.Join(dir, name)
	keep, err := check(path)
	if keep && err == nil {
		return nil
	}

	if removeErr := removeIfPresent(path); removeErr != nil {
		return fmt.Errorf("cache: revalidate: %w", errors.Join(err, removeErr))
	}
	if err != nil {
		return fmt.Errorf("cache: revalidate: %w", err)
	}

	return nil
}
