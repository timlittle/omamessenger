package cache_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// nameAfter returns a nameFor that names the file id+ext, recording how
// many times it was called.
func nameAfter(id, ext string, calls *int) func(string) (string, error) {
	return func(string) (string, error) {
		*calls++
		return id + ext, nil
	}
}

func TestFetchNamed_FillsOnceAndNamesByItsOwnContent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "media")
	c := cache.New(dir, 1<<20)
	fillCalls, nameCalls := 0, 0

	path, err := c.FetchNamed(t.Context(), "m1", writeBytes(10, &fillCalls), nameAfter("m1", ".pdf", &nameCalls))
	if err != nil || filepath.Base(path) != "m1.pdf" {
		t.Fatalf("FetchNamed = %q, %v; want m1.pdf", path, err)
	}

	again, err := c.FetchNamed(t.Context(), "m1", writeBytes(10, &fillCalls), nameAfter("m1", ".bin", &nameCalls))
	if err != nil || again != path || fillCalls != 1 || nameCalls != 1 {
		t.Errorf("second FetchNamed = %q, %v, fillCalls=%d nameCalls=%d; want the cached file with neither run again", again, err, fillCalls, nameCalls)
	}

	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want %o", p, info.Mode().Perm(), err, want)
		}
	}
}

// TestFetchNamed_NeverConfusesOneIDWithAnothersPrefix confirms "m1"
// never matches a cached file actually belonging to "m12": findByID
// requires the character right after the id to be a '.', not any
// other digit or letter that merely shares the same prefix.
func TestFetchNamed_NeverConfusesOneIDWithAnothersPrefix(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir(), 1<<20)
	fillCalls, nameCalls := 0, 0

	if _, err := c.FetchNamed(t.Context(), "m12", writeBytes(4, &fillCalls), nameAfter("m12", ".bin", &nameCalls)); err != nil {
		t.Fatal(err)
	}

	if _, err := c.FetchNamed(t.Context(), "m1", writeBytes(4, &fillCalls), nameAfter("m1", ".bin", &nameCalls)); err != nil {
		t.Fatal(err)
	}

	if fillCalls != 2 {
		t.Errorf("fillCalls = %d, want 2: \"m1\" must not have reused \"m12\"'s file", fillCalls)
	}
}

func TestFetchNamed_LeavesNothingWhenFillFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)
	broken := errors.New("offline")

	_, err := c.FetchNamed(t.Context(), "m1", func(_ context.Context, path string) error {
		_ = os.WriteFile(path, []byte("partial"), 0o600) // the failure below is what matters
		return broken
	}, nameAfter("m1", ".bin", new(int)))
	if !errors.Is(err, broken) {
		t.Fatalf("FetchNamed = %v, want the fill's error", err)
	}

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind %v", entries)
	}
}

func TestFetchNamed_LeavesNothingWhenNameForFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)
	broken := errors.New("unrecognised content")

	_, err := c.FetchNamed(t.Context(), "m1", writeBytes(4, new(int)), func(string) (string, error) {
		return "", broken
	})
	if !errors.Is(err, broken) {
		t.Fatalf("FetchNamed = %v, want nameFor's error", err)
	}

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind %v", entries)
	}
}

// TestFetchNamed_RejectsANameForResultOutsideTheCache confirms a name
// nameFor hands back is validated the same way Fetch's own upfront name
// is, so it can never escape the cache directory or collide with the
// ".part" suffix a fill in progress uses.
func TestFetchNamed_RejectsANameForResultOutsideTheCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)

	for _, name := range []string{"", "../x", "a/b", ".", "m1.part"} {
		_, err := c.FetchNamed(t.Context(), "m1", writeBytes(4, new(int)), func(string) (string, error) {
			return name, nil
		})
		if err == nil {
			t.Errorf("FetchNamed with nameFor returning %q succeeded", name)
		}
	}

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind %v", entries)
	}
}

func TestFetchNamed_RejectsIDsOutsideTheCache(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir(), 1<<20)
	for _, id := range []string{"", "../x", "a/b", "."} {
		if _, err := c.FetchNamed(t.Context(), id, writeBytes(4, new(int)), nameAfter(id, ".bin", new(int))); err == nil {
			t.Errorf("FetchNamed(%q) succeeded", id)
		}
	}
}

// TestFetchNamed_RefusesAFileLargerThanTheCacheOnItsOwn mirrors Fetch's
// own oversized check: FetchNamed must apply it too, not just Fetch.
func TestFetchNamed_RefusesAFileLargerThanTheCacheOnItsOwn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 25)

	_, err := c.FetchNamed(t.Context(), "huge", writeBytes(26, new(int)), nameAfter("huge", ".bin", new(int)))
	if !errors.Is(err, cache.ErrTooLargeForCache) {
		t.Errorf("FetchNamed() = %v, want ErrTooLargeForCache", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "huge.bin")); !os.IsNotExist(err) {
		t.Error("the oversized file was left in the cache")
	}
}

// TestFetchNamed_PrunesOverItsLimitTheSameAsFetch confirms a file named
// through FetchNamed still counts toward the cache's limit and is
// dropped by age the same as one named through Fetch.
func TestFetchNamed_PrunesOverItsLimitTheSameAsFetch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 15)

	kept, err := c.FetchNamed(t.Context(), "a", writeBytes(10, new(int)), nameAfter("a", ".bin", new(int)))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.FetchNamed(t.Context(), "b", writeBytes(10, new(int)), nameAfter("b", ".bin", new(int))); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Errorf("the least recently used file was kept: %v", err)
	}
}

// TestFetchNamed_SameIDFillsOnceUnderConcurrency mirrors
// TestFetch_SameNameFillsOnceUnderConcurrency for FetchNamed's own,
// separate lock namespace keyed by id rather than by final name.
func TestFetchNamed_SameIDFillsOnceUnderConcurrency(t *testing.T) {
	t.Parallel()

	c := cache.New(t.TempDir(), 1<<20)
	var calls atomic.Int32
	start := make(chan struct{})
	errs := make(chan error, 2)

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			_, err := c.FetchNamed(t.Context(), "m1", func(_ context.Context, path string) error {
				calls.Add(1)
				return os.WriteFile(path, []byte("a"), 0o600)
			}, nameAfter("m1", ".bin", new(int)))
			errs <- err
		})
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("FetchNamed error = %v", err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("fill calls = %d, want 1", n)
	}
}
