package app

// expiry.go sweeps away a disappearing message once its timer runs
// out: it is removed exactly like a user's own delete (its text, its
// media and the search entry the full-text index keeps alongside it),
// and the UI is told the same way a delete tells it, but its service
// is never asked to delete anything - the sender's own timer already
// did that on its own end. It is shaped like retry.go's scheduler: one
// goroutine for the life of the helper, re-arming itself for whichever
// expiry is soonest.

import (
	"context"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// expirySweeper removes every message whose disappearing timer has run
// out, as soon as it does.
type expirySweeper struct {
	store    *store.Store
	events   *events
	outgoing OutgoingMedia
	logger   Logger

	// changed wakes run's wait loop immediately, instead of leaving it
	// to its already-armed timer, whenever a message's expiry is newly
	// armed sooner than whatever it was already waiting for.
	changed wakeable
}

// newExpirySweeper prepares the sweeper. Call run once, from its own
// goroutine, to start it.
func newExpirySweeper(s *store.Store, events *events, outgoing OutgoingMedia, logger Logger) *expirySweeper {
	return &expirySweeper{store: s, events: events, outgoing: outgoing, logger: logger, changed: newWakeable()}
}

// noticeArmed wakes the sweeper to recompute when it should next fire,
// after a message's expiry is armed or re-read from a restart.
func (e *expirySweeper) noticeArmed() {
	e.changed.wake()
}

// run removes every message that is due, then waits until the next one
// falls due, a change wakes it early, or ctx is cancelled. It returns
// once ctx is done, which is the only way it stops.
func (e *expirySweeper) run(ctx context.Context) {
	for {
		next, ok, err := e.sweepDue(ctx)
		if err != nil {
			return // the store is gone; the helper is shutting down
		}

		if !waitUntil(ctx, e.changed, next, ok) {
			return
		}
	}
}

// sweepDue removes every message due now, then reports when the next
// one, if any, falls due so run knows how long to wait.
func (e *expirySweeper) sweepDue(ctx context.Context) (next time.Time, ok bool, err error) {
	pending, err := e.store.PendingExpiries(ctx)
	if err != nil {
		return time.Time{}, false, err
	}

	now := time.Now()
	for _, m := range pending {
		if time.UnixMilli(m.ExpiresAt).After(now) {
			break // PendingExpiries orders soonest due first
		}

		e.sweep(ctx, m)
	}

	return e.nextDue(ctx)
}

// nextDue reports the soonest due time among every expiry still
// pending, after sweepDue's own removals.
func (e *expirySweeper) nextDue(ctx context.Context) (next time.Time, ok bool, err error) {
	pending, err := e.store.PendingExpiries(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(pending) == 0 {
		return time.Time{}, false, nil
	}

	return time.UnixMilli(pending[0].ExpiresAt), true, nil
}

// sweep removes one due message and publishes its removal and its
// conversation's new summary, the same as a user's own delete, dropping
// its outgoing attachment too if it still has one.
func (e *expirySweeper) sweep(ctx context.Context, m domain.Message) {
	before := e.events.unreadTotal(ctx)

	deleted, err := e.store.DeleteMessagesByID(ctx, []string{m.ID})
	if err != nil || len(deleted) == 0 {
		return
	}

	for _, d := range deleted {
		e.events.publish(ctx, EventMessageRemoved, MessageRemoved{ConversationID: d.ConversationID, MessageID: d.ID})
		removeAttachment(ctx, e.outgoing, e.logger, d)
	}

	e.events.conversationChanged(ctx, m.ConversationID, before)
}
