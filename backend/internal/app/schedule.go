package app

import (
	"context"
	"time"
)

// wakeable is a single-slot wake-up signal, shared by the reminder and
// retry schedulers: each has its own timer for whatever is next due, but
// also needs an immediate nudge when something changes that timer out
// from under it (a reminder set sooner, an account reconnecting).
// Buffering one send is enough, since a pending wake-up already covers
// whatever triggered it.
type wakeable chan struct{}

// newWakeable creates a wakeable with room for one pending wake-up.
func newWakeable() wakeable {
	return make(wakeable, 1)
}

// wake queues a wake-up for waitUntil, unless one is queued already.
func (w wakeable) wake() {
	select {
	case w <- struct{}{}:
	default: // a wake-up is already pending; one is enough
	}
}

// waitUntil blocks until next arrives, w wakes, or ctx is cancelled,
// returning false only for the last of those. ok false means there is
// nothing due at all, so it waits only for a wake-up or cancellation.
func waitUntil(ctx context.Context, w wakeable, next time.Time, ok bool) bool {
	var fire <-chan time.Time
	if ok {
		timer := time.NewTimer(max(0, time.Until(next)))
		defer timer.Stop()
		fire = timer.C
	}

	select {
	case <-ctx.Done():
		return false
	case <-w:
		return true
	case <-fire:
		return true
	}
}
