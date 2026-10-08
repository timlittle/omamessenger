package app

import (
	"context"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// reminders wakes a snoozed conversation at its due time: a desktop
// notification and a conversation update, so the conversation list
// shows it again, marked, without the UI polling for it. It runs as one
// goroutine for the life of the helper, re-arming itself for whichever
// reminder is soonest, and waking at once when SetReminder adds or
// changes one that falls due sooner than whatever it was already
// waiting for.
type reminders struct {
	store    *store.Store
	notifier Notifier
	events   *events
	ui       *uiState

	// changed wakes run's wait loop immediately, instead of leaving it to
	// its already-armed timer, whenever a reminder is set or cleared.
	changed chan struct{}

	// fired remembers the due time already notified for each
	// conversation, so a reminder that stays set once it comes due (it
	// is not cleared automatically; see docs/decisions.md) is announced
	// once, not on every pass of run's loop. Touched only from run's own
	// goroutine, so it needs no lock.
	fired map[string]int64
}

// newReminders prepares a reminders scheduler. Call run once, from its
// own goroutine, to start it.
func newReminders(s *store.Store, notifier Notifier, events *events, ui *uiState) *reminders {
	return &reminders{store: s, notifier: notifier, events: events, ui: ui, changed: make(chan struct{}, 1), fired: map[string]int64{}}
}

// notifyChanged wakes run's wait loop to recompute when it should next
// fire, after a reminder is set or cleared.
func (r *reminders) notifyChanged() {
	select {
	case r.changed <- struct{}{}:
	default: // a wake-up is already pending; one is enough
	}
}

// run fires every reminder that is due, then waits until the next one
// falls due, a change wakes it early, or ctx is cancelled. It returns
// once ctx is done, which is the only way it stops.
func (r *reminders) run(ctx context.Context) {
	for {
		next, ok, err := r.fireDue(ctx)
		if err != nil {
			return // the store is gone; the helper is shutting down
		}

		if !r.wait(ctx, next, ok) {
			return
		}
	}
}

// fireDue notifies and republishes every reminder that is due now,
// reporting when the next one, if any, falls due so run knows how long
// to wait.
func (r *reminders) fireDue(ctx context.Context) (next time.Time, ok bool, err error) {
	pending, err := r.store.PendingReminders(ctx)
	if err != nil {
		return time.Time{}, false, err
	}

	now := time.Now()
	for _, conv := range pending {
		due := time.UnixMilli(conv.ReminderAt)
		if due.After(now) {
			return due, true, nil // PendingReminders orders soonest first
		}

		if r.fired[conv.ID] == conv.ReminderAt {
			continue // already notified for this exact due time
		}

		r.fire(ctx, conv)
		r.fired[conv.ID] = conv.ReminderAt
	}

	return time.Time{}, false, nil
}

// fire raises the due reminder's desktop notification and republishes
// its conversation, so the list picks it up without polling the helper.
func (r *reminders) fire(ctx context.Context, conv domain.Conversation) {
	settings, _, _ := r.ui.snapshot()
	title, body, convID := policy.ReminderNotification(settings.detail(), conv.Title, conv.ID)
	r.notifier.Notify(title, body, convID)
	r.events.publish(ctx, EventConversationUpdated, conv)
}

// wait blocks until next arrives, a reminder changes, or ctx is
// cancelled, returning false only for the last of those. ok false means
// there is nothing pending at all, so it waits only for a change.
func (r *reminders) wait(ctx context.Context, next time.Time, ok bool) bool {
	var fire <-chan time.Time
	if ok {
		timer := time.NewTimer(max(0, time.Until(next)))
		defer timer.Stop()
		fire = timer.C
	}

	select {
	case <-ctx.Done():
		return false
	case <-r.changed:
		return true
	case <-fire:
		return true
	}
}
