package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// Event names sent to the UI.
const (
	EventAccountUpdated      = "account.updated"
	EventConversationUpdated = "conversation.updated"
	EventMessageAdded        = "message.added"
	EventMessageUpdated      = "message.updated"
	EventMessageRemoved      = "message.removed"
	EventUnreadChanged       = "unread.changed"
	EventTyping              = "typing"
	EventAccountRemoved      = "account.removed"
	EventAuthStep            = "auth.step"
	EventNotificationClicked = "notification.clicked"
	EventScheduledAdded      = "scheduledMessage.added"
	EventScheduledUpdated    = "scheduledMessage.updated"
	EventScheduledRemoved    = "scheduledMessage.removed"
	EventNotificationSound   = "notification.sound"
	EventAccountNotice       = "account.notice"
)

// ScheduledRemoved is the data of a scheduledMessage.removed event: a
// queued message that was cancelled, sent now, or just came due,
// identified by its own id and the conversation it belonged to, since
// by the time the UI sees this the row itself is already gone.
type ScheduledRemoved struct {
	ConversationID string `json:"conversationId"`
	ScheduledID    string `json:"scheduledId"`
}

// UnreadChanged is the data of an unread.changed event. ByService is the
// unread total for each service that has any, so the bar tooltip can
// show WhatsApp and Telegram apart; it is empty, and omitted, once
// nothing anywhere is unread.
type UnreadChanged struct {
	Total     int            `json:"total"`
	ByService map[string]int `json:"byService,omitempty"`
}

// MessageRemoved is the data of a message.removed event.
type MessageRemoved struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
}

// Typing is the data of a typing event.
type Typing struct {
	ConversationID string `json:"conversationId"`
	Name           string `json:"name"`
	Active         bool   `json:"active"`
}

// AccountRemoved is the data of an account.removed event.
type AccountRemoved struct {
	AccountID string `json:"accountId"`
}

// AccountView is the data of an account.updated event, and
// accounts.list's own per-account shape: a stored domain.Account
// enriched with its connector's current attachment limit, for the
// composer's "Up to …" hint before a file is picked. Every place that
// publishes account.updated uses this, not a bare domain.Account, so a
// reconnect's own update - which WhatsApp's connector fires far more
// often than Telegram's - never silently wipes the limit the UI
// already had for the account (see viewAccount).
type AccountView struct {
	domain.Account
	AttachmentLimit int64 `json:"attachmentLimit"`
}

// viewAccount wraps account with limiter's current attachment limit
// for it (see AccountView), the same lookup and fallback
// AttachmentLimit itself runs, so every account.updated event reports
// exactly what accounts.list would for the same account right now.
func viewAccount(limiter AttachmentLimiter, account domain.Account) AccountView {
	return AccountView{Account: account, AttachmentLimit: attachmentLimitFor(limiter, account.ID, domain.MediaFile)}
}

// NotificationSound is the data of a notification.sound event: a hint,
// sent only once Ingest has already decided to show a desktop
// notification for an arriving message (so a muted, missed or
// already-read one never sends this at all), that the UI should decide
// whether to play a sound for conversationID. It carries no sender
// name, text or file path - only the service, so the UI knows which of
// its own manifest settings to weigh alongside the conversation's own
// choice (see ui/lib/NotificationSound.js).
type NotificationSound struct {
	ConversationID string `json:"conversationId"`
	Service        string `json:"service"`
	HasCustomSound bool   `json:"hasCustomSound"`
}

// AccountNotice is the data of an account.notice event: a safe,
// one-time heads-up about something an account's service cannot do
// right now, shown in the UI's existing toast rather than any error
// banner - WhatsApp's own app-state verification permanently failing
// for pin, archive, mark-read and delete-for-me is the only case
// today (see connector.ErrOrganizeLocalOnly and docs/decisions.md).
// Commands publishes it once per account per helper run, the first
// time it discovers the condition (see organizeLocalOnlyHandled).
type AccountNotice struct {
	AccountID string `json:"accountId"`
	Message   string `json:"message"`
}

// AuthStep is the data of an auth.step event: what an account's sign-in
// needs from the user next.
type AuthStep struct {
	AccountID    string `json:"accountId"`
	Kind         string `json:"kind"`
	QR           string `json:"qr,omitempty"`
	Hint         string `json:"hint,omitempty"`
	PasswordHint string `json:"passwordHint,omitempty"`
}

// NotificationClicked is the data of a notification.clicked event: the
// conversation a desktop notification carried, which clicking it should
// open.
type NotificationClicked struct {
	ConversationID string `json:"conversationId"`
}

// events publishes UI events, including the derived ones: a conversation's
// new summary and the unread total when it changes.
type events struct {
	store *store.Store
	out   Publisher
}

// publish sends one event.
func (e *events) publish(ctx context.Context, event string, data any) {
	e.out.Publish(ctx, event, data)
}

// unreadTotal returns the unread total, or 0 when it cannot be read; the
// next successful change corrects the badge.
func (e *events) unreadTotal(ctx context.Context) int {
	total, err := e.store.UnreadTotal(ctx)
	if err != nil {
		return 0
	}

	return total
}

// unreadChanged builds an unread.changed event's data for total, filling
// in the per-service breakdown or leaving it nil when it cannot be read;
// the next successful change corrects it.
func (e *events) unreadChanged(ctx context.Context, total int) UnreadChanged {
	byService, err := e.store.UnreadByService(ctx)
	if err != nil {
		return UnreadChanged{Total: total}
	}

	return UnreadChanged{Total: total, ByService: byService}
}

// conversationChanged publishes the conversation's current summary, then
// the unread total if it differs from before.
func (e *events) conversationChanged(ctx context.Context, id string, unreadBefore int) {
	conv, err := e.store.Conversation(ctx, id)
	if err != nil {
		return
	}

	e.publish(ctx, EventConversationUpdated, conv)

	if after := e.unreadTotal(ctx); after != unreadBefore {
		e.publish(ctx, EventUnreadChanged, e.unreadChanged(ctx, after))
	}
}

// publishConversation re-reads a conversation after a change to it and
// publishes its current summary, returning that summary so a caller that
// needs it, such as to hand it to a connector, does not have to read it
// again itself.
func (e *events) publishConversation(ctx context.Context, id string) (domain.Conversation, error) {
	conv, err := e.store.Conversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}

	e.publish(ctx, EventConversationUpdated, conv)

	return conv, nil
}
