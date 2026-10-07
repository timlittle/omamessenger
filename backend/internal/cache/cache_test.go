package cache_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// writeBytes returns a fill that writes n bytes and counts its calls.
func writeBytes(n int, calls *int) func(context.Context, string) error {
	return func(_ context.Context, path string) error {
		*calls++
		return os.WriteFile(path, make([]byte, n), 0o600)
	}
}

func TestFetch_FillsOnceAndKeepsFilesPrivate(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "media")
	c := cache.New(dir, 1<<20)
	calls := 0

	path, err := c.Fetch(t.Context(), "m1.jpg", writeBytes(10, &calls))
	if err != nil || filepath.Dir(path) != dir {
		t.Fatalf("Fetch = %q, %v", path, err)
	}

	if again, err := c.Fetch(t.Context(), "m1.jpg", writeBytes(10, &calls)); err != nil || again != path || calls != 1 {
		t.Errorf("second Fetch = %q, %v after %d fills; want the cached file", again, err, calls)
	}

	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want %o", p, info.Mode().Perm(), err, want)
		}
	}
}

func TestFetch_LeavesNothingWhenFillFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)
	broken := errors.New("offline")

	_, err := c.Fetch(t.Context(), "m1.jpg", func(_ context.Context, path string) error {
		_ = os.WriteFile(path, []byte("partial"), 0o600) // the failure below is what matters
		return broken
	})
	if !errors.Is(err, broken) {
		t.Fatalf("Fetch = %v, want the fill's error", err)
	}

	if entries, _ := os.ReadDir(dir); len(entries) != 0 { // a missing dir reads as empty too
		t.Errorf("left behind %v", entries)
	}
}

func TestFetch_DropsTheLeastRecentlyUsedOverTheLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 25)
	calls := 0

	first, _ := c.Fetch(t.Context(), "a", writeBytes(10, &calls))
	second, _ := c.Fetch(t.Context(), "b", writeBytes(10, &calls))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Fetch(t.Context(), "c", writeBytes(10, &calls)); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("the least recently used file was kept: %v", err)
	}

	if _, err := os.Stat(second); err != nil {
		t.Errorf("a recent file was dropped: %v", err)
	}
}

func TestFetch_RejectsNamesOutsideTheCache(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir(), 1<<20)
	for _, name := range []string{"", "../x", "a/b", "."} {
		if _, err := c.Fetch(t.Context(), name, func(context.Context, string) error { return nil }); err == nil {
			t.Errorf("Fetch(%q) succeeded", name)
		}
	}
}

func TestFetch_ReportsFilesystemFailures(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]*cache.Cache{
		"a directory that cannot be made": cache.New(filepath.Join(blocker, "media"), 1<<20),
		"a fill that wrote nothing":       cache.New(t.TempDir(), 1<<20),
	} {
		if _, err := c.Fetch(t.Context(), "m1.jpg", func(context.Context, string) error { return nil }); err == nil {
			t.Errorf("%s: Fetch succeeded", name)
		}
	}
}
