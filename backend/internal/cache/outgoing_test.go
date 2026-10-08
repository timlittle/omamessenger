package cache_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

func TestOutgoingStore_CopiesAndFindsAgain(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30, alwaysExists)

	path, err := o.Store(t.Context(), "m1", "photo.jpg", bytes.NewReader([]byte("image bytes")))
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if want := o.Path("m1", "photo.jpg"); path != want {
		t.Errorf("Store() = %q, want %q", path, want)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "image bytes" {
		t.Errorf("stored content = %q, %v, want %q", got, err, "image bytes")
	}

	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, %v; want %o", p, info.Mode().Perm(), err, want)
		}
	}
}

func TestOutgoingStore_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := o.Store(ctx, "m1", "file.bin", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Errorf("Store(cancelled ctx) error = %v, want context.Canceled", err)
	}
}

func TestOutgoingStore_FailsWhenTheDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	// A plain file where the outgoing directory should be makes
	// os.MkdirAll fail, since it cannot turn a file into a directory.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	o := cache.NewOutgoing(filepath.Join(blocked, "outgoing"), 24*time.Hour, 1<<30, alwaysExists)
	if _, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x")); err == nil {
		t.Error("Store() with a blocked directory = nil error, want one")
	}
}

func TestOutgoingStore_FailsWhenTheDestinationCannotBeOpened(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30, alwaysExists)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x")); err == nil {
		t.Error("Store() into a read-only directory = nil error, want one")
	}
}

func TestOutgoingStore_LeavesNothingWhenTheReaderFails(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30, alwaysExists)

	failing := errors.New("read failed")
	_, err := o.Store(t.Context(), "m1", "file.bin", failingReader{err: failing})
	if !errors.Is(err, failing) {
		t.Fatalf("Store() error = %v, want %v", err, failing)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("outgoing dir after a failed store = %v, %v; want it empty", entries, err)
	}
}

func TestOutgoingPath_NamesByIDAndFileName(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing("/data/outgoing", 24*time.Hour, 1<<30, alwaysExists)
	got := o.Path("m42", "cat.png")
	if !strings.HasSuffix(got, "m42-cat.png") || filepath.Dir(got) != "/data/outgoing" {
		t.Errorf("Path() = %q, want it under the directory, named by id and file name", got)
	}
}

// TestOutgoingRemove_DeletesAStoredCopy confirms Remove drops the file
// Store put there, for a message now confirmed delivered.
func TestOutgoingRemove_DeletesAStoredCopy(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	path, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}

	if err := o.Remove(t.Context(), "m1", "file.bin"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stat after Remove = %v, want it gone", err)
	}
}

// TestOutgoingRemove_ToleratesACopyAlreadyGone confirms removing a copy
// that was never stored, or already removed, is not an error: a
// delivery receipt can arrive more than once.
func TestOutgoingRemove_ToleratesACopyAlreadyGone(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	if err := o.Remove(t.Context(), "missing", "file.bin"); err != nil {
		t.Errorf("Remove() of a copy never stored = %v, want nil", err)
	}
}

// TestOutgoingRemove_RefusesACancelledContext confirms Remove checks
// ctx before touching the filesystem, the same as Store.
func TestOutgoingRemove_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := o.Remove(ctx, "m1", "file.bin"); !errors.Is(err, context.Canceled) {
		t.Errorf("Remove(cancelled ctx) error = %v, want context.Canceled", err)
	}
}

// TestOutgoingRemove_ReportsAFilesystemFailure confirms a removal
// failure that is not "already gone" - a non-empty directory sitting
// where the file should be, say - is reported rather than swallowed.
func TestOutgoingRemove_ReportsAFilesystemFailure(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	path := o.Path("m1", "file.bin")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "inside"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := o.Remove(t.Context(), "m1", "file.bin"); err == nil {
		t.Error("Remove() of a non-empty directory succeeded")
	}
}

// TestOutgoingStats_ReportsBytesHeldAndTheLimit mirrors Cache.Stats: the
// bytes actually stored against the limit the area was given, and an
// area never created yet is simply empty.
func TestOutgoingStats_ReportsBytesHeldAndTheLimit(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 100, alwaysExists)

	if bytes, limit, err := o.Stats(); err != nil || bytes != 0 || limit != 100 {
		t.Fatalf("Stats before any store = %d, %d, %v; want 0, 100, nil", bytes, limit, err)
	}

	if _, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("0123456789")); err != nil {
		t.Fatal(err)
	}

	if bytes, limit, err := o.Stats(); err != nil || bytes != 10 || limit != 100 {
		t.Errorf("Stats after one store = %d, %d, %v; want 10, 100, nil", bytes, limit, err)
	}
}

// TestOutgoingStats_ReportsFilesystemFailures confirms a Stats failure
// that is not "the area was never created" is reported, the same as
// Cache.Stats.
func TestOutgoingStats_ReportsFilesystemFailures(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), 24*time.Hour, 1<<30, alwaysExists)
	if _, _, err := o.Stats(); err == nil {
		t.Error("Stats() over a blocked path succeeded")
	}
}

func TestOutgoing_KeepsACraftedFileNameInsideItsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "media", "outgoing")
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := cache.NewOutgoing(dir, time.Hour, 1<<30, alwaysExists)

	for _, name := range []string{"/../../../victim.txt", "../../victim.txt", "a/../../../victim.txt"} {
		if got := o.Path("m1", name); filepath.Dir(got) != dir {
			t.Errorf("Path(%q) = %q, want a file directly inside %q", name, got, dir)
		}
		if err := o.Remove(t.Context(), "m1", name); err != nil {
			t.Fatalf("Remove(%q): %v", name, err)
		}
	}

	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a crafted attachment name removed a file outside the outgoing area: %v", err)
	}
}
