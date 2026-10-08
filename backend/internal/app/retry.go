package app

// retry.go retries a failed outgoing message automatically: at once
// when its account reconnects, and otherwise on a backoff, until it is
// sent, the user deletes it, or its failure classifies as permanent. It
// mirrors reminder.go's own goroutine: one for the life of the helper,
// started from main.go and re-armed from the store after a restart.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// retryBackoff is how long to wait before each automatic retry attempt
// of a failure streak: 30 seconds, 2 minutes, 10 minutes, then an hour,
// repeating hourly once exhausted.
var retryBackoff = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	time.Hour,
}

// retryBound is how long a failure streak keeps retrying automatically,
// from its first failure, before it is left for the user's own retry.
const retryBound = 24 * time.Hour

// retrier retries every failed outgoing message automatically: woken at
// once when its account reconnects, and otherwise whenever its own
// backoff comes due. It runs as one goroutine for the life of the
// helper, re-arming itself for whichever retry is soonest, the same way
// reminders does for snoozed conversations.
type retrier struct {
	store    *store.Store
	commands *Commands // filled in by New once Commands exists

	// changed wakes run's wait loop immediately, instead of leaving it
	// to its already-armed timer, whenever an account reconnects.
	changed wakeable

	mu    sync.Mutex
	woken map[string]bool // account ids reconnected since the last pass
}

// newRetrier prepares the automatic retry scheduler. Call run once,
// from its own goroutine, to start it.
func newRetrier(s *store.Store) *retrier {
	return &retrier{store: s, changed: newWakeable(), woken: map[string]bool{}}
}

// accountReconnected tells the scheduler to retry accountID's failed
// messages at once, regardless of their own backoff.
func (r *retrier) accountReconnected(accountID string) {
	r.mu.Lock()
	r.woken[accountID] = true
	r.mu.Unlock()

	r.changed.wake()
}

// takeWoken returns the accounts reconnected since the last pass and
// forgets them.
func (r *retrier) takeWoken() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	woken := r.woken
	r.woken = map[string]bool{}

	return woken
}

// run retries every failed message that is due, or whose account just
// reconnected, then waits until the next one falls due, a reconnect
// wakes it early, or ctx is cancelled. It returns once ctx is done,
// which is the only way it stops.
func (r *retrier) run(ctx context.Context) {
	for {
		next, ok, err := r.passDue(ctx)
		if err != nil {
			return // the store is gone; the helper is shutting down
		}

		if !waitUntil(ctx, r.changed, next, ok) {
			return
		}
	}
}

// passDue retries every pending retry that is due now, or belongs to an
// account that just reconnected, then reports when the next one, if
// any, falls due. It re-reads the pending list after attempting, since
// an attempt that failed again reschedules itself - possibly sooner
// than anything already in the list run started with - and run must
// wait for that new time, not a snapshot taken before it existed.
func (r *retrier) passDue(ctx context.Context) (next time.Time, ok bool, err error) {
	woken := r.takeWoken()

	pending, err := r.store.PendingRetries(ctx)
	if err != nil {
		return time.Time{}, false, err
	}

	now := time.Now()
	for _, m := range pending {
		conv, err := r.store.Conversation(ctx, m.ConversationID)
		if err != nil {
			continue
		}

		due := time.UnixMilli(m.RetryAt)
		if due.After(now) && !woken[conv.AccountID] {
			continue
		}

		_, _ = r.commands.retryAttempt(ctx, conv, m) // the message's own stored status is the result that matters
	}

	return r.nextDue(ctx)
}

// nextDue reports the soonest due time among every retry still pending,
// after passDue's own attempts have possibly rescheduled some of them.
func (r *retrier) nextDue(ctx context.Context) (next time.Time, ok bool, err error) {
	pending, err := r.store.PendingRetries(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(pending) == 0 {
		return time.Time{}, false, nil
	}

	return time.UnixMilli(pending[0].RetryAt), true, nil // PendingRetries orders soonest first
}

// retryAttempt sends m again, through a retry asked for or the automatic
// scheduler: it restores the attachment, if any, flips the message back
// to pending and dispatches it, then schedules or stops the next
// automatic attempt based on what the service said this time. m's own
// RetryAttempts and RetrySince decide whether this continues an
// existing backoff or starts a fresh one; Retry resets both to zero
// first so asking for a retry always starts the streak over.
func (c *Commands) retryAttempt(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	if err := c.restoreAttachment(ctx, &m); err != nil {
		if c.logger != nil {
			c.logger.Printf("retry: attachment unavailable, stopped")
		}
		_ = c.store.StopMessageRetry(ctx, m.ID)
		return m, err
	}

	pending, err := c.setStatus(ctx, m, domain.StatusPending)
	if err != nil {
		return pending, err
	}
	if m.Media != nil && pending.Media != nil {
		pending.Media.Path = m.Media.Path
	}

	return c.dispatchAndSchedule(ctx, conv, pending)
}

// dispatchAndSchedule hands m to the service and, on a refusal,
// schedules its next automatic retry or stops altogether, based on how
// the refusal classifies and m's own RetryAttempts and RetrySince.
func (c *Commands) dispatchAndSchedule(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	updated, sendErr, storeErr := c.send(ctx, conv, m)
	if storeErr != nil {
		return updated, storeErr
	}

	if sendErr == nil {
		_ = c.store.ClearMessageRetry(ctx, m.ID)
		return updated, nil
	}

	c.rescheduleOrStop(ctx, m, sendErr)

	return updated, nil
}

// rescheduleOrStop schedules m's next automatic retry under the
// backoff, honouring a longer wait the service itself asked for, or
// stops altogether when sendErr classifies as permanent or the failure
// streak has already run for retryBound.
func (c *Commands) rescheduleOrStop(ctx context.Context, m domain.Message, sendErr error) {
	if errors.Is(sendErr, connector.ErrSendPermanent) {
		if c.logger != nil {
			c.logger.Printf("retry: permanent failure, stopped")
		}
		_ = c.store.StopMessageRetry(ctx, m.ID)
		return
	}

	since := m.RetrySince
	if since == 0 {
		since = time.Now().UnixMilli()
	}

	if time.Since(time.UnixMilli(since)) >= retryBound {
		_ = c.store.StopMessageRetry(ctx, m.ID)
		return
	}

	delay := nextBackoff(m.RetryAttempts)
	if after, ok := connector.RetryAfter(sendErr); ok && after > delay {
		delay = after
	}

	at := time.Now().Add(delay).UnixMilli()
	_ = c.store.ScheduleMessageRetry(ctx, m.ID, at, m.RetryAttempts+1, since)
}

// nextBackoff returns the delay before the automatic retry numbered
// attempts+1 (1-based), holding at retryBackoff's last step once it is
// exhausted.
func nextBackoff(attempts int) time.Duration {
	if attempts < len(retryBackoff) {
		return retryBackoff[attempts]
	}

	return retryBackoff[len(retryBackoff)-1]
}
