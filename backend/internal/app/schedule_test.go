// wakeable and waitUntil are unexported scheduling primitives with no
// public entry point of their own - reminders and retrier are the only
// callers, and both are already covered through RunReminders and
// RunRetries. This file drives them directly instead, since there is no
// way to reach their edge cases (coalescing, waiting with nothing due)
// through the package's public API.
package app

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestWaitUntil_WakesImmediatelyOnWakeUp confirms a pending wake-up
// returns waitUntil at once, even though nothing it was waiting for was
// ever due.
func TestWaitUntil_WakesImmediatelyOnWakeUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		w := newWakeable()

		var wg sync.WaitGroup
		result := make(chan bool, 1)
		wg.Go(func() { result <- waitUntil(ctx, w, time.Time{}, false) })

		w.wake()
		wg.Wait()

		if got := <-result; !got {
			t.Errorf("waitUntil() = %v, want true", got)
		}
	})
}

// TestWaitUntil_ReturnsFalseOnCancel confirms waitUntil stops and
// reports false once ctx is cancelled, whether or not anything was due.
func TestWaitUntil_ReturnsFalseOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		w := newWakeable()

		var wg sync.WaitGroup
		result := make(chan bool, 1)
		wg.Go(func() { result <- waitUntil(ctx, w, time.Now().Add(time.Hour), true) })

		cancel()
		wg.Wait()

		if got := <-result; got {
			t.Errorf("waitUntil() = %v, want false", got)
		}
	})
}

// TestWaitUntil_FiresWhenNextArrives confirms waitUntil waits until next
// arrives when nothing wakes it early, and not a moment sooner.
func TestWaitUntil_FiresWhenNextArrives(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		w := newWakeable()
		next := time.Now().Add(time.Minute)

		var wg sync.WaitGroup
		result := make(chan bool, 1)
		wg.Go(func() { result <- waitUntil(ctx, w, next, true) })
		synctest.Wait()

		select {
		case <-result:
			t.Fatal("waitUntil returned before next arrived")
		default:
		}

		time.Sleep(time.Minute)
		wg.Wait()

		if got := <-result; !got {
			t.Errorf("waitUntil() = %v, want true", got)
		}
	})
}

// TestWaitUntil_WaitsOnlyForAWakeUpWhenNothingIsDue confirms waitUntil
// never returns on its own when ok is false, however long it waits -
// only a wake-up or cancellation can end it.
func TestWaitUntil_WaitsOnlyForAWakeUpWhenNothingIsDue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		w := newWakeable()

		var wg sync.WaitGroup
		result := make(chan bool, 1)
		wg.Go(func() { result <- waitUntil(ctx, w, time.Time{}, false) })

		time.Sleep(24 * time.Hour)
		synctest.Wait()

		select {
		case <-result:
			t.Fatal("waitUntil returned with nothing due and no wake-up")
		default:
		}

		w.wake()
		wg.Wait()

		if got := <-result; !got {
			t.Errorf("waitUntil() = %v, want true", got)
		}
	})
}

// TestWakeable_WakeCoalescesPendingWakeUps confirms a second wake-up
// queued before the first is read adds nothing further to wait for:
// one pending wake-up is enough.
func TestWakeable_WakeCoalescesPendingWakeUps(t *testing.T) {
	w := newWakeable()

	w.wake()
	w.wake()

	select {
	case <-w:
	default:
		t.Fatal("want a pending wake-up")
	}

	select {
	case <-w:
		t.Fatal("want only one pending wake-up, got a second")
	default:
	}
}
