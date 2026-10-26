package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RevalidateCheck decides what Revalidate should do with one file
// already in a media area: true to leave it in place, unchanged or
// already rewritten by check itself; false to have Revalidate remove
// it. An error means path could not even be read, which Revalidate
// reports but still treats like any other file to move past, rather
// than stopping the rest of the sweep.
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
// removes it when check reports keep=false.
func revalidateEntry(dir string, e os.DirEntry, check RevalidateCheck) error {
	name := e.Name()
	if !e.Type().IsRegular() || strings.HasSuffix(name, partSuffix) || name == formatMarkerName {
		return nil
	}

	path := filepath.Join(dir, name)
	keep, err := check(path)
	if err != nil {
		return fmt.Errorf("cache: revalidate: %w", err)
	}
	if keep {
		return nil
	}

	return removeIfPresent(path)
}
