package cache_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

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

	o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), 24*time.Hour, 1<<30, alwaysExists)
	if err := o.Sweep(t.Context()); err == nil {
		t.Error("Sweep() over a blocked path succeeded")
	}
}

// TestSweep_RemovesAnOrphanOlderThanItsGracePeriod confirms a copy
// backdated past the grace period, whose id matches no message at all,
// is swept, while one still within the grace period is kept even though
// it is equally orphaned - giving a copy just stored, such as a fresh
// clipboard paste not yet attached to any message, time to become one.
func TestSweep_RemovesAnOrphanOlderThanItsGracePeriod(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, neverExists)

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
		t.Errorf("an old orphan was kept: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("a fresh, not-yet-judged file was swept: %v", err)
	}
}

// TestSweep_NeverRemovesAPendingOrFailedMessagesCopy confirms a copy
// whose id matches a message the store still has - pending or failed,
// however old or over the area's size limit - is never removed: only
// Remove, once the message is sent or deleted, ends its life.
func TestSweep_NeverRemovesAPendingOrFailedMessagesCopy(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1, alwaysExists)

	path, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("a still-stored message's copy was removed: %v", err)
	}
}

// TestSweep_RemovesACopyOnceItsMessageIsGone confirms a copy whose
// message existed when it was stored, but no longer does by the time
// Sweep runs - the user deleted it - is removed as an orphan, once past
// the grace period.
func TestSweep_RemovesACopyOnceItsMessageIsGone(t *testing.T) {
	t.Parallel()

	messages := newFakeMessages("m1")
	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, messages.exists)

	path, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	messages.forget("m1")

	if err := o.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a deleted message's copy was kept: %v", err)
	}
}

// TestSweep_PropagatesAnExistsFailure confirms a failure checking
// whether a file's message exists is reported, rather than treated as
// an orphan to remove.
func TestSweep_PropagatesAnExistsFailure(t *testing.T) {
	t.Parallel()

	failing := errors.New("database closed")
	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30,
		func(context.Context, string) (bool, error) { return false, failing })

	path, err := o.Store(t.Context(), "m1", "file.bin", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, path, 2*time.Hour)

	if err := o.Sweep(t.Context()); !errors.Is(err, failing) {
		t.Errorf("Sweep() error = %v, want %v", err, failing)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("a file whose exists check failed was removed: %v", err)
	}
}

// TestSweep_NeverRemovesAFileReserved confirms a copy whose message is
// marked as being sent right now survives Sweep however old it is and
// however the exists check would answer.
func TestSweep_NeverRemovesAFileReserved(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, neverExists)

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
// Reserve gives ends once its release func is called, so an orphaned
// file is still eligible for the next sweep after its send ends.
func TestSweep_ReleasedReservationCanStillBeSwept(t *testing.T) {
	t.Parallel()

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, neverExists)

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

	o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, alwaysExists)
	if err := o.Sweep(t.Context()); err != nil {
		t.Errorf("Sweep() on a fresh area = %v, want nil", err)
	}
}

// TestSweep_ToleratesAFileNameWithoutAHyphen confirms a stray orphaned
// file with no id prefix - never something Store itself would create -
// is still swept like any other orphan, rather than panicking or
// jamming the sweep.
func TestSweep_ToleratesAFileNameWithoutAHyphen(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "outgoing")
	o := cache.NewOutgoing(dir, time.Hour, 1<<30, neverExists)
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
		t.Errorf("a stray, unprefixed orphan was kept: %v", err)
	}
}

// TestRunSweeper_SweepsOnStartupAndOnEveryTick confirms RunSweeper sweeps
// once right away, then again on each tick of its interval, stopping
// once ctx is cancelled. synctest fakes time, so this runs instantly
// instead of waiting on a real clock.
func TestRunSweeper_SweepsOnStartupAndOnEveryTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1<<30, neverExists)
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
			t.Fatalf("the startup sweep did not remove an orphan: %v", err)
		}

		second, err := o.Store(t.Context(), "second", "file.bin", strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}
		backdate(t, second, 2*time.Hour)

		time.Sleep(time.Minute)
		synctest.Wait()

		if _, err := os.Stat(second); !os.IsNotExist(err) {
			t.Errorf("a tick did not sweep an orphan: %v", err)
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
		o := cache.NewOutgoing(filepath.Join(blocker, "outgoing"), time.Hour, 1<<30, alwaysExists)
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
