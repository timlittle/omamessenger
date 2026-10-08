package app

import (
	"context"
	"sync"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// markReadDebounce delays telling a conversation's service it was read
// while the user keeps chatting, so a burst of arriving messages produces
// one MarkRead call instead of one per message.
const markReadDebounce = 500 * time.Millisecond

// markReadTimeout bounds the debounced MarkRead call, so a connector that
// never answers cannot leave the timer's goroutine running forever.
const markReadTimeout = 10 * time.Second

// Ingest stores updates from connectors and publishes them to the UI.
// Updates for unknown accounts or conversations are dropped: a connector
// reports a conversation before its messages. Storage errors are dropped
// too, because a connector has no way to act on them; the next update
// brings the UI up to date.
type Ingest struct {
	store      *store.Store
	notifier   Notifier
	dispatcher Dispatcher
	outgoing   OutgoingMedia
	cache      MediaCache
	logger     Logger
	events     *events
	ui         *uiState

	mu            sync.Mutex
	pendingRead   *time.Timer
	pendingConv   domain.Conversation
	pendingUnread int
}

var (
	_ connector.Sink        = (*Ingest)(nil)
	_ connector.SenderNamer = (*Ingest)(nil)
	_ connector.PollUpdater = (*Ingest)(nil)
)

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

// Incoming records a live message, counting it towards the conversation's
// unread total. If the user is looking at its conversation it is read at
// once, locally and, debounced, with the service, so a message read
// while chatting never leaves the service's own count non-zero to
// resurrect the badge later; otherwise it may notify.
func (in *Ingest) Incoming(ctx context.Context, accountID, conversationRemoteID string, m domain.Message) {
	conv, m, before, ok := in.save(ctx, accountID, conversationRemoteID, m, true)
	if !ok {
		return
	}

	arrival := in.arrival(conv, m)
	if policy.MarkReadOnArrival(arrival) {
		_, _ = in.store.MarkRead(ctx, conv.ID) // see the Ingest comment on dropped errors
		in.scheduleMarkRead(conv, 1)
	}

	in.events.publish(ctx, EventMessageAdded, m)
	in.events.conversationChanged(ctx, conv.ID, before)

	if policy.ShouldNotify(arrival) {
		in.notifier.Notify(policy.Notification(arrival))
	}
}

// History records an earlier message: the first open of a conversation
// whose history was not yet paged locally, or scrolling further back in
// one already open. It never notifies, is never read on arrival, and
// never counts towards the unread total, which the service's own count
// (synced separately through Unread) already covers; otherwise a chat
// opened for the first time would have every backfilled message bump it,
// racing whatever marked it read at the same time.
func (in *Ingest) History(ctx context.Context, accountID, conversationRemoteID string, m domain.Message) {
	conv, m, before, ok := in.save(ctx, accountID, conversationRemoteID, m, false)
	if !ok {
		return
	}

	in.events.publish(ctx, EventMessageAdded, m)
	in.events.conversationChanged(ctx, conv.ID, before)
}

// Edited updates a stored message's text, media and reactions after the
// service reports it changed. It never notifies; the open conversation
// refreshes the message in place.
func (in *Ingest) Edited(ctx context.Context, accountID, conversationRemoteID string, m domain.Message) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	updated, found, err := in.store.EditMessage(ctx, conv.ID, m.RemoteID, store.MessageEdit{
		Text: m.Text, Media: m.Media, Reactions: m.Reactions, Mentions: m.Mentions, MentionsMe: m.MentionsMe,
	})
	if err != nil || !found {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, updated)
}

// Reacted updates a stored message's reaction chips after the service
// reports they changed on their own, without a full edit. It never
// notifies; the open conversation refreshes the message in place.
func (in *Ingest) Reacted(ctx context.Context, accountID, conversationRemoteID, messageRemoteID string, reactions []domain.Reaction) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	updated, found, err := in.store.SetReactions(ctx, conv.ID, messageRemoteID, reactions)
	if err != nil || !found {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, updated)
}

// Deleted removes stored messages and publishes a removal for each one,
// plus the conversations they left changed and the unread total if it
// moved. It never notifies.
func (in *Ingest) Deleted(ctx context.Context, accountID string, conversationRemoteIDs, remoteIDs []string) {
	before := in.events.unreadTotal(ctx)

	deleted, err := in.store.DeleteMessages(ctx, accountID, conversationRemoteIDs, remoteIDs)
	if err != nil || len(deleted) == 0 {
		return
	}

	for _, m := range deleted {
		in.events.publish(ctx, EventMessageRemoved, MessageRemoved{ConversationID: m.ConversationID, MessageID: m.ID})
	}

	for _, conv := range distinctConversations(deleted) {
		in.events.conversationChanged(ctx, conv, before)
		before = in.events.unreadTotal(ctx)
	}
}

// distinctConversations lists the conversation ids messages belonged to,
// each once, in first-seen order.
func distinctConversations(messages []domain.Message) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if seen[m.ConversationID] {
			continue
		}

		seen[m.ConversationID] = true
		ids = append(ids, m.ConversationID)
	}

	return ids
}

// Unread takes the service's unread count for a conversation, publishing
// it only when it changed. A non-zero count for the conversation the user
// is looking at right now is never shown: it means the service has not
// caught up with a read reported while its debounce was still pending, or
// a sync race, so it is marked read again, locally and with the service,
// rather than left to resurrect the badge.
func (in *Ingest) Unread(ctx context.Context, accountID, conversationRemoteID string, count int) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	if count > 0 && in.looking(conv.ID) {
		before := in.events.unreadTotal(ctx)
		if changed, err := in.store.MarkRead(ctx, conv.ID); err == nil && changed {
			in.events.conversationChanged(ctx, conv.ID, before)
		}
		in.scheduleMarkRead(conv, count)
		return
	}

	before := in.events.unreadTotal(ctx)
	if changed, err := in.store.SetUnread(ctx, conv.ID, count); err != nil || !changed {
		return
	}

	in.events.conversationChanged(ctx, conv.ID, before)
}

// Organized records a conversation's pinned and archived state, from a
// dialog sync, publishing it only when it changed.
func (in *Ingest) Organized(ctx context.Context, accountID, conversationRemoteID string, pinned, archived bool) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	changed, err := in.store.SetOrganized(ctx, conv.ID, pinned, archived)
	if err != nil || !changed {
		return
	}

	updated, err := in.store.Conversation(ctx, conv.ID)
	if err != nil {
		return
	}

	in.events.publish(ctx, EventConversationUpdated, updated)
}

// OutgoingStatus records the service's id for a sent message and
// publishes its delivery progress. Late or out-of-order receipts are
// ignored. Once delivery is confirmed (sent, delivered or read), the
// message will never be retried, so its outgoing attachment, if any, is
// dropped (see retireOutgoingAttachment).
func (in *Ingest) OutgoingStatus(ctx context.Context, localMessageID, remoteID, status string) {
	if remoteID != "" {
		_ = in.store.SetMessageRemoteID(ctx, localMessageID, remoteID) // see the Ingest comment on dropped errors
	}

	m, changed, err := in.store.UpdateMessageStatus(ctx, localMessageID, status)
	if err != nil || !changed {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, m)
	in.retireOutgoingAttachment(ctx, m)
}

// Typing publishes a typing indicator for a known conversation.
func (in *Ingest) Typing(ctx context.Context, accountID, conversationRemoteID, name string, active bool) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	in.events.publish(ctx, EventTyping, Typing{ConversationID: conv.ID, Name: name, Active: active})
}

// SenderName corrects senderRemoteID's name on every message already
// stored under a different one, and re-publishes every conversation
// whose preview this changes, so a group whose preview still names
// its newest message's sender by a stale, generic label picks up a
// contact, push or business name that only resolved afterwards (see
// connector.SenderNamer).
func (in *Ingest) SenderName(ctx context.Context, accountID, senderRemoteID, name string) {
	changed, err := in.store.RefreshSenderName(ctx, accountID, senderRemoteID, name)
	if err != nil {
		return
	}

	for _, id := range changed {
		conv, err := in.store.Conversation(ctx, id)
		if err != nil {
			continue
		}

		in.events.publish(ctx, EventConversationUpdated, conv)
	}
}

// AuthStep publishes what an account's sign-in needs from the user.
func (in *Ingest) AuthStep(ctx context.Context, accountID string, step connector.AuthStep) {
	in.events.publish(ctx, EventAuthStep, AuthStep{AccountID: accountID, Kind: step.Kind, QR: step.QR, Hint: step.Hint})
}

// save stores a message in a known conversation, counting it towards the
// unread total only when countUnread is set. ok is false when the
// conversation is unknown, storing fails or the message is a duplicate.
// before is the unread total before the message was stored.
func (in *Ingest) save(ctx context.Context, accountID, remoteID string, m domain.Message, countUnread bool) (_ domain.Conversation, _ domain.Message, before int, ok bool) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, remoteID)
	if err != nil {
		return conv, m, 0, false
	}

	m.ConversationID = conv.ID
	if m.Created == 0 {
		m.Created = time.Now().UnixMilli()
	}

	before = in.events.unreadTotal(ctx)
	add := in.store.AddMessage
	if !countUnread {
		add = in.store.AddHistoryMessage
	}
	m, inserted, err := add(ctx, m)

	return conv, m, before, err == nil && inserted
}

// looking reports whether the user is focused on conversationID with the
// window active right now.
func (in *Ingest) looking(conversationID string) bool {
	_, focused, windowActive := in.ui.snapshot()

	return policy.MarkReadOnArrival(policy.Input{WindowActive: windowActive, Focused: focused == conversationID})
}

// scheduleMarkRead reports conv as read to its service after
// markReadDebounce, extending the wait if another message arrives first
// so a burst produces one call rather than one per message. unread adds
// to the count flushMarkRead finally reports: conv's own Unread field is
// stale by the time this runs, already cleared locally by the MarkRead
// call that precedes it, so accumulating here is the only way the
// service ends up told how many messages the whole burst actually left
// it to send a read receipt for.
func (in *Ingest) scheduleMarkRead(conv domain.Conversation, unread int) {
	in.mu.Lock()
	defer in.mu.Unlock()

	in.pendingConv = conv
	in.pendingUnread += unread
	if in.pendingRead != nil {
		in.pendingRead.Reset(markReadDebounce)
		return
	}

	in.pendingRead = time.AfterFunc(markReadDebounce, in.flushMarkRead)
}

// flushMarkRead sends the debounced MarkRead call for the most recently
// scheduled conversation, on its own background context: nothing in
// whichever call triggered the schedule survives the wait. With read
// receipts off, the local clear this debounce followed already stands on
// its own, so the service is never told; see Settings.ReadReceipts.
func (in *Ingest) flushMarkRead() {
	in.mu.Lock()
	conv := in.pendingConv
	conv.Unread = in.pendingUnread
	in.pendingRead = nil
	in.pendingUnread = 0
	in.mu.Unlock()

	settings, _, _ := in.ui.snapshot()
	if !settings.ReadReceipts {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), markReadTimeout)
	defer cancel()

	_ = in.dispatcher.MarkRead(ctx, conv) // best effort; a later Unread sync corrects any miss
}

// arrival describes a live message arriving under the current UI state.
func (in *Ingest) arrival(conv domain.Conversation, m domain.Message) policy.Input {
	settings, focused, windowActive := in.ui.snapshot()

	return policy.Input{
		Notifications:  settings.Notifications,
		Detail:         settings.detail(),
		Muted:          conv.Muted,
		Focused:        focused == conv.ID,
		WindowActive:   windowActive,
		Kind:           conv.Kind,
		Sender:         m.SenderName,
		Title:          conv.Title,
		Text:           m.Text,
		ConversationID: conv.ID,
	}
}
