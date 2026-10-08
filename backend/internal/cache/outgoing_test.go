package cache_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// newOutgoing returns an Outgoing over a fresh temp directory with a
// retention and size limit generous enough never to interfere with a
// test that is not itself about sweeping.
func newOutgoing(t *testing.T) *cache.Outgoing {
	t.Helper()

	return cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30)
}

func TestOutgoingStore_CopiesAndFindsAgain(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30)

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

	o := cache.NewOutgoing(filepath.Join(blocked, "outgoing"), 24*time.Hour, 1<<30)
	if _, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x")); err == nil {
		t.Error("Store() with a blocked directory = nil error, want one")
	}
}

func TestOutgoingStore_FailsWhenTheDestinationCannotBeOpened(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30)
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
	o := cache.NewOutgoing(dir, 24*time.Hour, 1<<30)

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

	o := cache.NewOutgoing("/data/outgoing", 24*time.Hour, 1<<30)
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

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 100)

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

	o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), 24*time.Hour, 1<<30)
	if _, _, err := o.Stats(); err == nil {
		t.Error("Stats() over a blocked path succeeded")
	}
}

// TestSweep_RefusesACancelledContext confirms Sweep checks ctx before
// touching the filesystem.
func TestSweep_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := o.Sweep(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Sweep(cancelled ctx) error = %v, want context.Canceled", err)
	}
}

// TestSweep_ReportsFilesystemFailures confirms a Sweep failure that is
// not "the area was never created" is reported.
func TestSweep_ReportsFilesystemFailures(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), 24*time.Hour, 1<<30)
	if err := o.Sweep(t.Context()); err == nil {
		t.Error("Sweep() over a blocked path succeeded")
	}
}

// TestSweep_RemovesCopiesOlderThanRetention confirms a copy backdated
// past the retention window is swept, and one still within it is kept.
func TestSweep_RemovesCopiesOlderThanRetention(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30)

	old, err := o.Store(t.Context(), "old", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := o.Store(t.Context(), "fresh", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}

	backdate(t, old, 2*time.Hour)

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("an expired copy was kept: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("a fresh copy was swept: %v", err)
	}
}

// TestSweep_EvictsLeastRecentlyUsedOverTheSizeLimit confirms that once
// the area holds more than its size limit, Sweep drops the oldest
// copies first, even though none of them are old enough for the
// retention window alone to remove.
func TestSweep_EvictsLeastRecentlyUsedOverTheSizeLimit(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 25)

	first, err := o.Store(t.Context(), "a", "file.bin", strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := o.Store(t.Context(), "b", "file.bin", strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, first, time.Hour)

	if _, err := o.Store(t.Context(), "c", "file.bin", strings.NewReader("0123456789")); err != nil {
		t.Fatal(err)
	}

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("the least recently used copy was kept: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Errorf("a more recently used copy was evicted: %v", err)
	}
}

// TestSweep_NeverRemovesAFileReserved confirms a copy whose message is
// marked as being sent right now survives both retention and the size
// limit, however old or over quota the area is.
func TestSweep_NeverRemovesAFileReserved(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1)

	path, err := o.Store(t.Context(), "sending", "file.bin", strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	release := o.Reserve("sending")
	defer release()

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("a reserved file was removed: %v", err)
	}
}

// TestSweep_ReleasedReservationCanStillBeSwept confirms the protection
// Reserve gives ends once its release func is called, so a file is
// still eligible for the next sweep after its send ends.
func TestSweep_ReleasedReservationCanStillBeSwept(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30)

	path, err := o.Store(t.Context(), "sent", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	release := o.Reserve("sent")
	release()

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a released file was kept: %v", err)
	}
}

// TestSweep_ToleratesAnAreaNeverCreated confirms sweeping an outgoing
// area with nothing ever stored in it is not an error.
func TestSweep_ToleratesAnAreaNeverCreated(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30)
	if err := o.Sweep(t.Context()); err != nil {
		t.Errorf("Sweep() on a fresh area = %v, want nil", err)
	}
}

// TestRunSweeper_SweepsOnStartupAndOnEveryTick confirms RunSweeper sweeps
// once right away, then again on each tick of its interval, stopping
// once ctx is cancelled. synctest fakes time, so this runs instantly
// instead of waiting on a real clock.
func TestRunSweeper_SweepsOnStartupAndOnEveryTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30)
		ctx, cancel := context.WithCancel(t.Context())

		path, err := o.Store(t.Context(), "old", "file.bin", strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}
		backdate(t, path, 2*time.Hour)

		done := make(chan struct{})
		go func() {
			o.RunSweeper(ctx, time.Minute, nil)
			close(done)
		}()
		synctest.Wait()

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("the startup sweep did not remove an expired copy: %v", err)
		}

		second, err := o.Store(t.Context(), "second", "file.bin", strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}
		backdate(t, second, 2*time.Hour)

		time.Sleep(time.Minute)
		synctest.Wait()

		if _, err := os.Stat(second); !os.IsNotExist(err) {
			t.Errorf("a tick did not sweep an expired copy: %v", err)
		}

		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("RunSweeper did not stop once its context was cancelled")
		}
	})
}

// TestRunSweeper_LogsAFailedSweep confirms a sweep that fails - the
// area blocked by a file where its directory should be, here - is
// logged both on the immediate startup sweep and on a later tick,
// rather than stopping RunSweeper.
func TestRunSweeper_LogsAFailedSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), time.Hour, 1<<30)
		log := &recordingLogger{}
		ctx, cancel := context.WithCancel(t.Context())

		go o.RunSweeper(ctx, time.Minute, log)
		synctest.Wait()

		if n := log.count(); n != 1 {
			t.Fatalf("logged lines after startup = %d, want 1", n)
		}

		time.Sleep(time.Minute)
		synctest.Wait()

		if n := log.count(); n != 2 {
			t.Errorf("logged lines after one tick = %d, want 2", n)
		}

		cancel()
		synctest.Wait()
	})
}

// TestSweep_ToleratesAFileNameWithoutAHyphen confirms a stray file with
// no id prefix - never something Store itself would create - is still
// swept like any other expired file, rather than panicking or jamming
// the sweep.
func TestSweep_ToleratesAFileNameWithoutAHyphen(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir, time.Hour, 1<<30)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "stray")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a stray, unprefixed file was kept: %v", err)
	}
}

// recordingLogger counts the diagnostic lines it is given, standing in
// for a *log.Logger without a real writer to parse.
type recordingLogger struct {
	mu sync.Mutex
	n  int
}

func (l *recordingLogger) Printf(string, ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.n++
}

func (l *recordingLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.n
}

// backdate sets path's modification time back by d, so a sweep sees it
// as older than it actually is.
func backdate(t *testing.T, path string, d time.Duration) {
	t.Helper()

	old := time.Now().Add(-d)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

// failingReader always fails to read, like a clipboard command that
// could not be run.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }
