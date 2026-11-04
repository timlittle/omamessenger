package app

// scheduled.go lets the user queue a text message to send at a chosen
// time. A queued message lives in its own store table (see
// store/scheduled.go); once due, or sent early, it is claimed and
// handed to Commands.Send as an ordinary outgoing message.
//
// scheduledSender mirrors reminders and retrier: one goroutine for the
// life of the helper, re-arming for whichever message is soonest due
// and woken early on any change.

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// scheduledSender sends every queued message once its own time comes.
type scheduledSender struct {
	store    *store.Store
	commands *Commands // filled in by New once Commands exists

	// changed wakes run's wait loop immediately, instead of leaving it
	// to its already-armed timer, whenever a queued message is added,
	// edited or cancelled.
	changed wakeable
}

// newScheduledSender prepares the scheduled-send runner. Call run
// once, from its own goroutine, to start it.
func newScheduledSender(s *store.Store) *scheduledSender {
	return &scheduledSender{store: s, changed: newWakeable()}
}

// notifyChanged wakes run's wait loop to recompute when it should next
// fire, after a queued message is added, edited or cancelled.
func (sc *scheduledSender) notifyChanged() {
	sc.changed.wake()
}

// run sends every queued message that is due, then waits until the
// next one falls due, a change wakes it early, or ctx is cancelled. It
// returns once ctx is done, which is the only way it stops.
func (sc *scheduledSender) run(ctx context.Context) {
	for {
		next, ok, err := sc.fireDue(ctx)
		if err != nil {
			return // the store is gone; the helper is shutting down
		}

		if !waitUntil(ctx, sc.changed, next, ok) {
			return
		}
	}
}

// fireDue claims and sends every queued message due now, reporting
// when the next one falls due so run knows how long to wait. A
// message already claimed or cancelled by the time this tries it is
// simply skipped.
func (sc *scheduledSender) fireDue(ctx context.Context) (next time.Time, ok bool, err error) {
	pending, err := sc.store.ScheduledMessages(ctx)
	if err != nil {
		return time.Time{}, false, err
	}

	now := time.Now()
	for _, m := range pending {
		due := time.UnixMilli(m.SendAt)
		if due.After(now) {
			return due, true, nil // ScheduledMessages orders soonest first
		}

		claimed, err := sc.store.ClaimScheduledMessage(ctx, m.ID)
		if err != nil {
			return time.Time{}, false, err
		}
		if !claimed {
			continue
		}

		_, _ = sc.commands.dispatchScheduled(ctx, m) // a failed send is already recorded as a failed message; there is nothing further to do with the error here
	}

	return time.Time{}, false, nil
}

// dispatchScheduled publishes that a claimed message is no longer
// pending, then hands it to the normal send path, where a refusal
// becomes a failed message rather than an error here.
func (c *Commands) dispatchScheduled(ctx context.Context, m domain.ScheduledMessage) (domain.Message, error) {
	c.events.publish(ctx, EventScheduledRemoved, ScheduledRemoved{ConversationID: m.ConversationID, ScheduledID: m.ID})

	sent, err := c.Send(ctx, m.ConversationID, m.Text, SendOptions{})
	if err != nil && c.logger != nil {
		c.logger.Printf("scheduled message: send failed")
	}

	return sent, err
}

// ScheduleMessage queues text to send to conversationID at sendAt
// (Unix milliseconds), which must be in the future, then wakes the
// sender so it recomputes when to wake next.
func (c *Commands) ScheduleMessage(ctx context.Context, conversationID, text string, sendAt int64) (domain.ScheduledMessage, error) {
	text, err := validateScheduledText(text)
	if err != nil {
		return domain.ScheduledMessage{}, err
	}
	if err := validateScheduledTime(sendAt); err != nil {
		return domain.ScheduledMessage{}, err
	}

	if _, err := c.store.Conversation(ctx, conversationID); err != nil {
		return domain.ScheduledMessage{}, err
	}

	m, err := c.store.AddScheduledMessage(ctx, domain.ScheduledMessage{ConversationID: conversationID, Text: text, SendAt: sendAt})
	if err != nil {
		return domain.ScheduledMessage{}, err
	}

	c.events.publish(ctx, EventScheduledAdded, m)
	c.scheduled.notifyChanged()

	return m, nil
}

// ListScheduled lists queued messages, soonest due first: every one
// queued when conversationID is "", or only its own otherwise, for a
// chat's own pending list and the "Scheduled messages" command alike.
func (c *Commands) ListScheduled(ctx context.Context, conversationID string) ([]domain.ScheduledMessage, error) {
	all, err := c.store.ScheduledMessages(ctx)
	if err != nil {
		return nil, err
	}
	if conversationID == "" {
		return all, nil
	}

	filtered := make([]domain.ScheduledMessage, 0, len(all))
	for _, m := range all {
		if m.ConversationID == conversationID {
			filtered = append(filtered, m)
		}
	}

	return filtered, nil
}

// EditScheduled changes a queued message's text and send time, both
// validated the same way ScheduleMessage validates a new one, failing
// with domain.ErrNotFound once it has already been sent or cancelled.
func (c *Commands) EditScheduled(ctx context.Context, id, text string, sendAt int64) (domain.ScheduledMessage, error) {
	text, err := validateScheduledText(text)
	if err != nil {
		return domain.ScheduledMessage{}, err
	}
	if err := validateScheduledTime(sendAt); err != nil {
		return domain.ScheduledMessage{}, err
	}

	m, err := c.store.UpdateScheduledMessage(ctx, id, text, sendAt)
	if err != nil {
		return domain.ScheduledMessage{}, err
	}

	c.events.publish(ctx, EventScheduledUpdated, m)
	c.scheduled.notifyChanged()

	return m, nil
}

// CancelScheduled removes a queued message before it is due, failing
// with domain.ErrNotFound once it has already been sent.
func (c *Commands) CancelScheduled(ctx context.Context, id string) error {
	m, err := c.store.ScheduledMessage(ctx, id)
	if err != nil {
		return err
	}

	if err := c.store.CancelScheduledMessage(ctx, id); err != nil {
		return err
	}

	c.events.publish(ctx, EventScheduledRemoved, ScheduledRemoved{ConversationID: m.ConversationID, ScheduledID: id})
	c.scheduled.notifyChanged()

	return nil
}

// SendScheduledNow claims a queued message and sends it right away
// instead of waiting for its own time, failing with domain.ErrNotFound
// if it was already sent or cancelled.
func (c *Commands) SendScheduledNow(ctx context.Context, id string) (domain.Message, error) {
	m, err := c.store.ScheduledMessage(ctx, id)
	if err != nil {
		return domain.Message{}, err
	}

	claimed, err := c.store.ClaimScheduledMessage(ctx, id)
	if err != nil {
		return domain.Message{}, err
	}
	if !claimed {
		return domain.Message{}, domain.ErrNotFound
	}

	return c.dispatchScheduled(ctx, m)
}

// RunScheduledMessages sends every queued message once its own time
// comes, until ctx is cancelled. main.go starts it as its own
// goroutine, alongside RunReminders and RunRetries.
func (c *Commands) RunScheduledMessages(ctx context.Context) {
	c.scheduled.run(ctx)
}

// validateScheduledText trims and checks text the same way Send's own
// captionText does: required, within the connector's length limit.
// Attachments are not supported for a scheduled message yet (see
// docs/decisions.md).
func validateScheduledText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%w: text is required", ErrInvalidInput)
	}
	if utf8.RuneCountInString(text) > domain.MaxTextLength {
		return "", domain.ErrTextTooLong
	}

	return text, nil
}

// validateScheduledTime checks that sendAt (Unix milliseconds) is in
// the future, the one thing that tells a message worth queuing apart
// from one that should just be sent now.
func validateScheduledTime(sendAt int64) error {
	if sendAt <= time.Now().UnixMilli() {
		return fmt.Errorf("%w: sendAt must be in the future", ErrInvalidInput)
	}

	return nil
}
