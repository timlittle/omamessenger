package app_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// storeExists adapts db's MessageExists to cache.MessageExists, for an
// outgoing media area built in a test to tell a real orphan from a
// message still stored.
func storeExists(db *store.Store) cache.MessageExists {
	return func(ctx context.Context, id string) (bool, error) { return db.MessageExists(ctx, id) }
}

// setErr changes the error fakeDispatcher.Send returns, safe to call
// while another goroutine - the retry scheduler, say - may be calling
// Send concurrently.
func (d *fakeDispatcher) setErr(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.err = err
}

// sendCount reports how many times fakeDispatcher.Send has been called
// so far.
func (d *fakeDispatcher) sendCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.sent)
}

// runRetries starts f.commands.RunRetries in its own goroutine,
// returning a function that cancels it and waits for it to stop, so
// every test leaves no goroutine running past its own end.
func runRetries(t *testing.T, ctx context.Context, cancel context.CancelFunc, f *fixture) func() {
	t.Helper()

	var wg sync.WaitGroup
	wg.Go(func() { f.commands.RunRetries(ctx) })

	return func() {
		cancel()
		wg.Wait()
	}
}

// TestRunRetries_ReconnectRetriesAtOnce confirms a failed message is
// retried the moment its account reconnects, rather than waiting for
// its own backoff to come due.
func TestRunRetries_ReconnectRetriesAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(errors.New("offline"))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}

		stop := runRetries(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		if n := f.dispatcher.sendCount(); n != 1 {
			t.Fatalf("sends before any reconnect or backoff = %d, want 1 (just the original)", n)
		}

		f.dispatcher.setErr(nil)
		f.ingest.AccountStatus(ctx, "wa", domain.AccountConnected, "")
		synctest.Wait()

		if n := f.dispatcher.sendCount(); n != 2 {
			t.Fatalf("sends after reconnecting = %d, want 2", n)
		}

		stored, err := f.store.Message(ctx, failed.ID)
		if err != nil || stored.Status != domain.StatusPending {
			t.Errorf("stored after reconnect = %+v, %v, want it retried and no longer failed", stored, err)
		}
	})
}

// TestRetry_RacingTheSchedulerSendsOnlyOnce reproduces the automatic
// retry scheduler's own pass and a user's manual Retry both reaching
// the same failed, due message at once - an account reconnecting the
// moment the user clicks retry, say. Only one of them must actually
// hand the message to the service.
func TestRetry_RacingTheSchedulerSendsOnlyOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(errors.New("offline"))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}
		f.dispatcher.setErr(nil) // the race's winner should now succeed
		baseline := f.dispatcher.sendCount()

		stop := runRetries(t, ctx, cancel, f)
		defer stop()

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = f.commands.Retry(ctx, failed.ID) })
		f.ingest.AccountStatus(ctx, "wa", domain.AccountConnected, "") // wakes the scheduler's own pass at once
		wg.Wait()
		synctest.Wait()

		if n := f.dispatcher.sendCount() - baseline; n != 1 {
			t.Fatalf("sends from the racing scheduler pass and manual retry = %d, want exactly 1", n)
		}

		stored, err := f.store.Message(ctx, failed.ID)
		if err != nil || stored.Status != domain.StatusPending {
			t.Errorf("stored after the race = %+v, %v, want pending", stored, err)
		}
	})
}

// TestRunRetries_FollowsTheBackoffSchedule confirms automatic retries
// follow the documented backoff - 30s, 2m, 10m, then hourly - and stop
// rescheduling once a retry finally succeeds.
func TestRunRetries_FollowsTheBackoffSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(errors.New("offline"))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}

		stop := runRetries(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		time.Sleep(29 * time.Second)
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 1 {
			t.Fatalf("sends just before the first backoff step = %d, want 1", n)
		}

		time.Sleep(time.Second) // 30s total
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 2 {
			t.Fatalf("sends after the 30s step = %d, want 2", n)
		}

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 3 {
			t.Fatalf("sends after the 2m step = %d, want 3", n)
		}

		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 4 {
			t.Fatalf("sends after the 10m step = %d, want 4", n)
		}

		f.dispatcher.setErr(nil)
		time.Sleep(time.Hour)
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 5 {
			t.Fatalf("sends after the 1h step = %d, want 5", n)
		}

		stored, err := f.store.Message(ctx, failed.ID)
		if err != nil || stored.Status != domain.StatusPending || stored.RetryAt != 0 {
			t.Errorf("stored once it finally succeeded = %+v, %v, want pending with nothing further scheduled", stored, err)
		}

		// Nothing more should be scheduled: waiting another hour sends
		// nothing further.
		time.Sleep(time.Hour)
		synctest.Wait()
		if n := f.dispatcher.sendCount(); n != 5 {
			t.Errorf("sends after it already succeeded = %d, want still 5", n)
		}
	})
}

// TestRunRetries_StopsOnAPermanentFailure confirms a send refusal that
// classifies as permanent - connector.ErrSendPermanent - is never
// scheduled for an automatic retry at all, and that a scheduler already
// running never retries it either, however long it waits.
func TestRunRetries_StopsOnAPermanentFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(fmt.Errorf("blocked: %w", connector.ErrSendPermanent))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}

		stored, err := f.store.Message(ctx, failed.ID)
		if err != nil || stored.RetryAt != 0 {
			t.Fatalf("stored after a permanent failure = %+v, %v, want nothing scheduled", stored, err)
		}

		stop := runRetries(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		time.Sleep(48 * time.Hour)
		synctest.Wait()

		if n := f.dispatcher.sendCount(); n != 1 {
			t.Errorf("sends after waiting = %d, want 1: a permanent failure must never retry", n)
		}
	})
}

// TestRunRetries_StopsAfterTheBound confirms a failure streak that
// keeps failing stops scheduling further automatic attempts once it has
// run for retryBound (24 hours) since it first failed, leaving the
// message for the user's own retry.
func TestRunRetries_StopsAfterTheBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(errors.New("offline"))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}

		stop := runRetries(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		time.Sleep(30 * time.Hour)
		synctest.Wait()

		stored, err := f.store.Message(ctx, failed.ID)
		if err != nil || stored.RetryAt != 0 {
			t.Fatalf("stored after the bound = %+v, %v, want nothing further scheduled", stored, err)
		}

		sends := f.dispatcher.sendCount()
		time.Sleep(2 * time.Hour)
		synctest.Wait()

		if n := f.dispatcher.sendCount(); n != sends {
			t.Errorf("sends after the bound has passed = %d, want still %d", n, sends)
		}
	})
}

// TestRetry_ResetsTheBackoff confirms a retry the user asks for starts
// the failure streak over: if it fails again, the next automatic retry
// waits the first backoff step, not wherever the streak had already
// reached.
func TestRetry_ResetsTheBackoff(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.dispatcher.err = errors.New("offline")

	failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	stored, err := f.store.Message(ctx, failed.ID)
	if err != nil || stored.RetryAttempts != 1 {
		t.Fatalf("RetryAttempts after the first failure = %+v, %v, want 1", stored, err)
	}

	retried, err := f.commands.Retry(ctx, failed.ID)
	if err != nil || retried.Status != domain.StatusFailed {
		t.Fatalf("Retry() = %+v, %v", retried, err)
	}

	stored, err = f.store.Message(ctx, failed.ID)
	if err != nil || stored.RetryAttempts != 1 {
		t.Errorf("RetryAttempts after a manual retry fails again = %+v, %v, want 1 (reset, not 2)", stored, err)
	}
}

// TestRunRetries_ReArmsAfterARestart confirms a retry still scheduled
// when the helper stops is found and retried after a restart - a
// second Commands and Ingest built over the same database, as main.go
// does after an actual process restart - rather than being lost.
func TestRunRetries_ReArmsAfterARestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.setErr(errors.New("offline"))

		failed, err := f.commands.Send(ctx, "chat", "hello", app.SendOptions{})
		if err != nil || failed.Status != domain.StatusFailed {
			t.Fatalf("Send() = %+v, %v", failed, err)
		}

		// The first helper run stops before the scheduled retry fires.
		stop := runRetries(t, ctx, cancel, f)
		synctest.Wait()
		stop()

		if n := f.dispatcher.sendCount(); n != 1 {
			t.Fatalf("sends before stopping = %d, want 1 (just the original)", n)
		}

		commands2, _, dispatcher2, _ := appOver(t, f.store)
		ctx2, cancel2 := context.WithCancel(t.Context())

		var wg sync.WaitGroup
		wg.Go(func() { commands2.RunRetries(ctx2) })
		t.Cleanup(func() { cancel2(); wg.Wait() })
		synctest.Wait()

		time.Sleep(30 * time.Second)
		synctest.Wait()

		if n := dispatcher2.sendCount(); n != 1 {
			t.Errorf("sends after restart and the first backoff step = %d, want 1", n)
		}

		stored, err := f.store.Message(ctx2, failed.ID)
		if err != nil || stored.Status != domain.StatusPending {
			t.Errorf("stored after the re-armed retry succeeded = %+v, %v, want pending", stored, err)
		}
	})
}

// TestRunRetries_StopsWhenContextIsCancelled confirms RunRetries
// returns once ctx is cancelled, leaking no goroutine.
func TestRunRetries_StopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())

		stop := runRetries(t, ctx, cancel, f)
		stop() // returns once RunRetries has actually stopped; no goroutine leak
	})
}
