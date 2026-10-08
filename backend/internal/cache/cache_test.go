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

// TestStats_ReportsBytesHeldAndTheLimit confirms Stats counts the bytes
// actually cached so far against the limit the cache was given, and that
// a cache directory never created yet is simply empty rather than an
// error.
func TestStats_ReportsBytesHeldAndTheLimit(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "media")
	c := cache.New(dir, 1<<20)
	calls := 0

	if bytes, limit, err := c.Stats(); err != nil || bytes != 0 || limit != 1<<20 {
		t.Fatalf("Stats before any fetch = %d, %d, %v; want 0, %d, nil", bytes, limit, err, int64(1<<20))
	}

	if _, err := c.Fetch(t.Context(), "m1.jpg", writeBytes(10, &calls)); err != nil {
		t.Fatal(err)
	}

	if bytes, limit, err := c.Stats(); err != nil || bytes != 10 || limit != 1<<20 {
		t.Errorf("Stats after one fetch = %d, %d, %v; want 10, %d, nil", bytes, limit, err, int64(1<<20))
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

// TestAdopt_MovesTheFileInAndServesItWithoutFilling confirms a file
// already on disk is moved into the cache under name, found afterwards
// without the fill func ever running.
func TestAdopt_MovesTheFileInAndServesItWithoutFilling(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "sent.jpg")
	if err := os.WriteFile(src, []byte("photo bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "media")
	c := cache.New(dir, 1<<20)

	if err := c.Adopt(t.Context(), "m1.jpg", src); err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file after Adopt = %v, want it moved away", err)
	}

	calls := 0
	path, err := c.Fetch(t.Context(), "m1.jpg", writeBytes(10, &calls))
	if err != nil || calls != 0 {
		t.Fatalf("Fetch() after Adopt = %q, %v, filled %d times; want no fill", path, err, calls)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "photo bytes" {
		t.Errorf("adopted content = %q, %v, want %q", got, err, "photo bytes")
	}
}

// TestAdopt_ToleratesASourceAlreadyGone confirms adopting a path that no
// longer exists is not an error: the file may already have been cleaned
// up elsewhere before Adopt ran.
func TestAdopt_ToleratesASourceAlreadyGone(t *testing.T) {
	t.Parallel()

	c := cache.New(filepath.Join(t.TempDir(), "media"), 1<<20)
	missing := filepath.Join(t.TempDir(), "gone.jpg")

	if err := c.Adopt(t.Context(), "m1.jpg", missing); err != nil {
		t.Errorf("Adopt() of a missing source = %v, want nil", err)
	}
}

// TestAdopt_PrunesOverItsLimitTheSameAsAFill confirms an adopted file
// counts towards the cache's limit, dropping the least recently used
// file the same as a normal fill would.
func TestAdopt_PrunesOverItsLimitTheSameAsAFill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 15)
	calls := 0

	kept, err := c.Fetch(t.Context(), "a", writeBytes(10, &calls))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(kept, old, old); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "b.bin")
	if err := os.WriteFile(src, make([]byte, 10), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := c.Adopt(t.Context(), "b", src); err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}

	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Errorf("the least recently used file was kept after Adopt: %v", err)
	}
}

// TestAdopt_RejectsNamesOutsideTheCache confirms Adopt validates name
// the same way Fetch does.
func TestAdopt_RejectsNamesOutsideTheCache(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := cache.New(t.TempDir(), 1<<20)
	for _, name := range []string{"", "../x", "a/b", "."} {
		if err := c.Adopt(t.Context(), name, src); err == nil {
			t.Errorf("Adopt(%q) succeeded", name)
		}
	}
}

// TestAdopt_RefusesACancelledContext confirms Adopt checks ctx before
// touching the filesystem, the same as Fetch.
func TestAdopt_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir(), 1<<20)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := c.Adopt(ctx, "m1.jpg", filepath.Join(t.TempDir(), "x.bin")); !errors.Is(err, context.Canceled) {
		t.Errorf("Adopt(cancelled ctx) error = %v, want context.Canceled", err)
	}
}

// TestAdopt_FailsWhenTheDirectoryCannotBeCreated mirrors
// TestFetch_ReportsFilesystemFailures for Adopt's own directory creation.
func TestAdopt_FailsWhenTheDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	c := cache.New(filepath.Join(blocker, "media"), 1<<20)
	if err := c.Adopt(t.Context(), "m1.jpg", src); err == nil {
		t.Error("Adopt() into a directory that cannot be made succeeded")
	}
}

// TestAdopt_FailsWhenTheDestinationIsNotAPlainFile confirms a rename
// failure that is not "the file is on another device" (os.Rename onto
// an existing directory, say) is reported rather than silently retried
// as a cross-device copy.
func TestAdopt_FailsWhenTheDestinationIsNotAPlainFile(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)
	if err := os.Mkdir(filepath.Join(dir, "m1.jpg"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := c.Adopt(t.Context(), "m1.jpg", src); err == nil {
		t.Error("Adopt() onto an existing directory succeeded")
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
