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
	"time"
)

// partSuffix marks a file still being filled, which is never served or
// counted.
const partSuffix = ".part"

// ErrInvalidName reports a name that is not a plain file name.
var ErrInvalidName = errors.New("cache: invalid file name")

// Cache is a size-limited directory of downloaded files. Fetches run one
// at a time, which keeps two fills of the same file from racing.
type Cache struct {
	dir   string
	limit int64

	mu sync.Mutex
}

// New returns a cache in dir that keeps at most limit bytes.
func New(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit}
}

// Fetch returns the path of the cached file name, first calling fill to
// write it when it is not cached. A fill that fails leaves nothing behind.
func (c *Cache) Fetch(ctx context.Context, name string, fill func(ctx context.Context, path string) error) (string, error) {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.HasSuffix(name, partSuffix) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.mu.Lock()
	defer c.mu.Unlock()

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

// prune drops the least recently used files until the cache fits its
// limit, never the file just fetched.
func (c *Cache) prune(keep string) error {
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
