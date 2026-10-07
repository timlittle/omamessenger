package cache_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

func TestOutgoingStore_CopiesAndFindsAgain(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir)

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

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"))
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

	o := cache.NewOutgoing(filepath.Join(blocked, "outgoing"))
	if _, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x")); err == nil {
		t.Error("Store() with a blocked directory = nil error, want one")
	}
}

func TestOutgoingStore_FailsWhenTheDestinationCannotBeOpened(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	o := cache.NewOutgoing(dir)
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
	o := cache.NewOutgoing(dir)

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

	o := cache.NewOutgoing("/data/outgoing")
	got := o.Path("m42", "cat.png")
	if !strings.HasSuffix(got, "m42-cat.png") || filepath.Dir(got) != "/data/outgoing" {
		t.Errorf("Path() = %q, want it under the directory, named by id and file name", got)
	}
}

// failingReader always fails to read, like a clipboard command that
// could not be run.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }
