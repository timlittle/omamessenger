// Package cache keeps downloaded media in one private directory, dropping
// the least recently used files once they pass a size limit.
package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// partSuffix marks a file still being filled, which is never served or
// counted.
const partSuffix = ".part"

// ErrInvalidName reports a name that is not a plain file name.
var ErrInvalidName = errors.New("cache: invalid file name")

// Cache is a size-limited directory of downloaded files. Two fetches for
// different names fill concurrently; two fetches for the same name share
// one lock, so only one of them actually fills it.
type Cache struct {
	dir   string
	limit int64

	keysMu sync.Mutex
	keys   map[string]*keyLock

	bookMu sync.Mutex // guards reading and pruning the directory, separately from any one file's fill
}

// keyLock is a per-file-name lock. It is reference counted so its entry
// in Cache.keys is removed once nobody holds or is waiting for it,
// rather than growing the map forever.
type keyLock struct {
	mu   sync.Mutex
	refs int
}

// New returns a cache in dir that keeps at most limit bytes.
func New(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit, keys: make(map[string]*keyLock)}
}

// lockName acquires the lock for name, creating it on first use, and
// returns a function that releases it. Only one caller at a time holds
// a given name's lock, so concurrent fills of the same file can never
// race, while different names never wait on each other.
func (c *Cache) lockName(name string) func() {
	c.keysMu.Lock()
	k, ok := c.keys[name]
	if !ok {
		k = &keyLock{}
		c.keys[name] = k
	}
	k.refs++
	c.keysMu.Unlock()

	k.mu.Lock()

	return func() {
		k.mu.Unlock()

		c.keysMu.Lock()
		k.refs--
		if k.refs == 0 {
			delete(c.keys, name)
		}
		c.keysMu.Unlock()
	}
}

// Fetch returns the path of the cached file name, first calling fill to
// write it when it is not cached. A fill that fails leaves nothing behind.
func (c *Cache) Fetch(ctx context.Context, name string, fill func(ctx context.Context, path string) error) (string, error) {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.HasSuffix(name, partSuffix) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}

	defer c.lockName(name)()

	path := filepath.Join(c.dir, name)
	if _, err := os.Stat(path); err == nil {
		now := time.Now()

		return path, os.Chtimes(path, now, now)
	}

	if err := c.fill(ctx, path, fill); err != nil {
		return "", err
	}

	return path, c.prune(name)
}

// Stats returns how many bytes the cache currently holds and the limit
// it was given, for a health check to compare. A cache directory not
// created yet (nothing has been fetched into it) simply holds zero
// bytes, not an error.
func (c *Cache) Stats() (bytes, limit int64, err error) {
	c.bookMu.Lock()
	defer c.bookMu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, c.limit, nil
		}

		return 0, c.limit, fmt.Errorf("cache: stats: %w", err)
	}

	_, total := cached(entries)

	return total, c.limit, nil
}

// fill writes path through a temporary file, so a failed or interrupted
// fill never looks cached.
func (c *Cache) fill(ctx context.Context, path string, fill func(ctx context.Context, path string) error) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("cache: %w", err)
	}

	part := path + partSuffix
	if err := fill(ctx, part); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	if err := os.Chmod(part, 0o600); err != nil {
		return fmt.Errorf("cache: %w", errors.Join(err, removeIfPresent(part)))
	}

	if err := os.Rename(part, path); err != nil {
		return fmt.Errorf("cache: %w", errors.Join(err, removeIfPresent(part)))
	}

	return nil
}

// Adopt places the file at path into the cache under name, so a file
// this helper already created elsewhere - an attachment just sent, say -
// is served and governed by this cache's own limit from here on,
// without being downloaded again. A path that no longer exists is not
// an error: by the time Adopt runs, the file may already be gone.
func (c *Cache) Adopt(ctx context.Context, name, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.HasSuffix(name, partSuffix) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}

	defer c.lockName(name)()

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("cache: adopt: %w", err)
	}

	dst := filepath.Join(c.dir, name)
	if err := adoptFile(path, dst); err != nil {
		return fmt.Errorf("cache: adopt: %w", err)
	}

	return c.prune(name)
}

// adoptFile places the file at src at dst: a rename when the two paths
// share a filesystem, the usual case here since both media areas live
// under the same data directory, or a copy across filesystems otherwise.
func adoptFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	return copyAcrossDevices(src, dst)
}

// copyAcrossDevices copies src to dst through a temporary file, so a
// failed or interrupted copy never leaves a partial file at dst, then
// removes src. adoptFile falls back to this only when the outgoing and
// cache areas turn out not to share a filesystem; in every real
// deployment they both live under the same data directory, so this has
// no practical way to exercise without actually mounting two
// filesystems.
func copyAcrossDevices(src, dst string) error { // coverage-ignore: needs two real filesystems to exercise; see the doc comment
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() // reading only; nothing to flush

	if err := copyToFile(dst, in); err != nil {
		return err
	}

	return removeIfPresent(src)
}

// prune drops the least recently used files until the cache fits its
// limit, never the file just fetched. It locks the directory bookkeeping
// separately from any file's per-name fill lock, so it never runs twice
// at once even while unrelated fills proceed concurrently.
func (c *Cache) prune(keep string) error {
	c.bookMu.Lock()
	defer c.bookMu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("cache: %w", err)
	}

	files, total := cached(entries)
	slices.SortFunc(files, func(a, b os.FileInfo) int { return a.ModTime().Compare(b.ModTime()) })

	var errs []error
	for _, f := range files {
		if total <= c.limit {
			break
		}

		if f.Name() == keep {
			continue
		}

		errs = append(errs, removeIfPresent(filepath.Join(c.dir, f.Name())))
		total -= f.Size()
	}

	return errors.Join(errs...)
}

// cached lists the finished files among entries and their total size.
func cached(entries []os.DirEntry) ([]os.FileInfo, int64) {
	var files []os.FileInfo
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || strings.HasSuffix(e.Name(), partSuffix) {
			continue
		}

		files = append(files, info)
		total += info.Size()
	}

	return files, total
}

// removeIfPresent removes path, treating one already gone as removed.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
