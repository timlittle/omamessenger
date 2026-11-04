package server

// scheduled.go answers the scheduled-message methods: queuing a message to
// send later, and managing what is still queued. See docs/api.md.

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// scheduledMethods are the scheduled-message entries of the protocol's method table.
func scheduledMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"messages.schedule":         bind(messagesSchedule(c)),
		"messages.listScheduled":    bind(messagesListScheduled(c)),
		"messages.editScheduled":    bind(messagesEditScheduled(c)),
		"messages.cancelScheduled":  bind(messagesCancelScheduled(c)),
		"messages.sendScheduledNow": bind(messagesSendScheduledNow(c)),
	}
}

// scheduleParams queues text to send to a conversation at sendAt
// (Unix milliseconds) instead of right away.
type scheduleParams struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
	SendAt         int64  `json:"sendAt"`
}

// messagesSchedule queues a message to send later.
func messagesSchedule(c *app.Commands) func(context.Context, scheduleParams) (any, error) {
	return func(ctx context.Context, p scheduleParams) (any, error) {
		return c.ScheduleMessage(ctx, p.ConversationID, p.Text, p.SendAt)
	}
}

// scheduledIDParams names one queued message, for cancelling or
// sending it right away.
type scheduledIDParams struct {
	ScheduledID string `json:"scheduledId"`
}

// messagesCancelScheduled cancels a queued message before it sends.
func messagesCancelScheduled(c *app.Commands) func(context.Context, scheduledIDParams) (any, error) {
	return func(ctx context.Context, p scheduledIDParams) (any, error) {
		return none{}, c.CancelScheduled(ctx, p.ScheduledID)
	}
}

// messagesSendScheduledNow sends a queued message right away, instead
// of waiting for its own time.
func messagesSendScheduledNow(c *app.Commands) func(context.Context, scheduledIDParams) (any, error) {
	return func(ctx context.Context, p scheduledIDParams) (any, error) {
		return c.SendScheduledNow(ctx, p.ScheduledID)
	}
}

// editScheduledParams changes a queued message's text and send time.
type editScheduledParams struct {
	ScheduledID string `json:"scheduledId"`
	Text        string `json:"text"`
	SendAt      int64  `json:"sendAt"`
}

// messagesEditScheduled changes a queued message's text and send time.
func messagesEditScheduled(c *app.Commands) func(context.Context, editScheduledParams) (any, error) {
	return func(ctx context.Context, p editScheduledParams) (any, error) {
		return c.EditScheduled(ctx, p.ScheduledID, p.Text, p.SendAt)
	}
}

// listScheduledParams filters the scheduled-message list to one
// conversation, or lists every one queued when ConversationID is "".
type listScheduledParams struct {
	ConversationID string `json:"conversationId,omitempty"`
}

// listScheduledResult is a list of queued messages, soonest due first.
type listScheduledResult struct {
	Messages []domain.ScheduledMessage `json:"messages"`
}

// messagesListScheduled lists queued messages.
func messagesListScheduled(c *app.Commands) func(context.Context, listScheduledParams) (any, error) {
	return func(ctx context.Context, p listScheduledParams) (any, error) {
		messages, err := c.ListScheduled(ctx, p.ConversationID)

		return listScheduledResult{Messages: messages}, err
	}
}
