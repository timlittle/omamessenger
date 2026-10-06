package app

import (
	"context"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// Ingest stores updates from connectors and publishes them to the UI.
// Updates for unknown accounts or conversations are dropped: a connector
// reports a conversation before its messages. Storage errors are dropped
// too, because a connector has no way to act on them; the next update
// brings the UI up to date.
type Ingest struct {
	store    *store.Store
	notifier Notifier
	events   *events
	ui       *uiState
}

var _ connector.Sink = (*Ingest)(nil)

// AccountStatus records and publishes an account's connection state.
func (in *Ingest) AccountStatus(ctx context.Context, accountID, status, detail string) {
	account, err := in.store.SetAccountStatus(ctx, accountID, status, detail)
	if err != nil {
		return
	}

	in.events.publish(ctx, EventAccountUpdated, account)
}

// Contact records a contact.
func (in *Ingest) Contact(ctx context.Context, c domain.Contact) {
	_ = in.store.UpsertContact(ctx, c) // see the Ingest comment on dropped errors
}

// Conversation records a conversation and publishes it.
func (in *Ingest) Conversation(ctx context.Context, c domain.Conversation) {
	conv, _, err := in.store.EnsureConversation(ctx, c)
	if err != nil {
		return
	}

	in.events.publish(ctx, EventConversationUpdated, conv)
}

// Incoming records a live message. If the user is looking at its
// conversation it is read at once; otherwise it may notify.
func (in *Ingest) Incoming(ctx context.Context, accountID, conversationRemoteID string, m domain.Message) {
	conv, m, before, ok := in.save(ctx, accountID, conversationRemoteID, m)
	if !ok {
		return
	}

	arrival := in.arrival(conv, m)
	if policy.MarkReadOnArrival(arrival) {
		_, _ = in.store.MarkRead(ctx, conv.ID) // see the Ingest comment on dropped errors
	}

	in.events.publish(ctx, EventMessageAdded, m)
	in.events.conversationChanged(ctx, conv.ID, before)

	if policy.ShouldNotify(arrival) {
		in.notifier.Notify(policy.Notification(arrival))
	}
}

// History records an earlier message. It never notifies and is never read
// on arrival.
func (in *Ingest) History(ctx context.Context, accountID, conversationRemoteID string, m domain.Message) {
	conv, m, before, ok := in.save(ctx, accountID, conversationRemoteID, m)
	if !ok {
		return
	}

	in.events.publish(ctx, EventMessageAdded, m)
	in.events.conversationChanged(ctx, conv.ID, before)
}

// OutgoingStatus records the service's id for a sent message and publishes
// its delivery progress. Late or out-of-order receipts are ignored.
func (in *Ingest) OutgoingStatus(ctx context.Context, localMessageID, remoteID, status string) {
	if remoteID != "" {
		_ = in.store.SetMessageRemoteID(ctx, localMessageID, remoteID) // see the Ingest comment on dropped errors
	}

	m, changed, err := in.store.UpdateMessageStatus(ctx, localMessageID, status)
	if err != nil || !changed {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, m)
}

// Typing publishes a typing indicator for a known conversation.
func (in *Ingest) Typing(ctx context.Context, accountID, conversationRemoteID, name string, active bool) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	in.events.publish(ctx, EventTyping, Typing{ConversationID: conv.ID, Name: name, Active: active})
}

// AuthStep publishes what an account's sign-in needs from the user.
func (in *Ingest) AuthStep(ctx context.Context, accountID string, step connector.AuthStep) {
	in.events.publish(ctx, EventAuthStep, AuthStep{AccountID: accountID, Kind: step.Kind, QR: step.QR, Hint: step.Hint})
}

// save stores a message in a known conversation. ok is false when the
// conversation is unknown, storing fails or the message is a duplicate.
// before is the unread total before the message was stored.
func (in *Ingest) save(ctx context.Context, accountID, remoteID string, m domain.Message) (_ domain.Conversation, _ domain.Message, before int, ok bool) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, remoteID)
	if err != nil {
		return conv, m, 0, false
	}

	m.ConversationID = conv.ID
	if m.Created == 0 {
		m.Created = time.Now().UnixMilli()
	}

	before = in.events.unreadTotal(ctx)
	m, inserted, err := in.store.AddMessage(ctx, m)

	return conv, m, before, err == nil && inserted
}

// arrival describes a live message arriving under the current UI state.
func (in *Ingest) arrival(conv domain.Conversation, m domain.Message) policy.Input {
	settings, focused, windowActive := in.ui.snapshot()

	return policy.Input{
		Notifications: settings.Notifications,
		Preview:       settings.NotificationPreview,
		Muted:         conv.Muted,
		Focused:       focused == conv.ID,
		WindowActive:  windowActive,
		Kind:          conv.Kind,
		Sender:        m.SenderName,
		Title:         conv.Title,
		Text:          m.Text,
	}
}
