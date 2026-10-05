package app

import (
	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Ingest receives normalized updates from connectors, persists them and
// publishes the resulting events. It implements connector.Sink and
// connector.HistorySink. Updates for unknown accounts or conversations are
// dropped: a connector reports a conversation before its messages.
type Ingest struct {
	repo     Repository
	pub      *publisher
	session  *session
	notifier Notifier
	clock    connector.Clock
}

var (
	_ connector.Sink        = (*Ingest)(nil)
	_ connector.HistorySink = (*Ingest)(nil)
)

// AccountStatus persists and publishes an account's connection state.
func (in *Ingest) AccountStatus(accountID, status, detail string) {
	if account, err := in.repo.SetAccountStatus(accountID, status, detail); err == nil {
		in.pub.send("account.updated", account)
	}
}

// Contact persists a normalized contact.
func (in *Ingest) Contact(contact domain.Contact) {
	_ = in.repo.UpsertContact(contact)
}

// Conversation upserts a normalized conversation and publishes it.
func (in *Ingest) Conversation(conversation domain.Conversation) {
	if updated, _, err := in.repo.EnsureConversation(conversation); err == nil {
		in.pub.send("conversation.updated", updated)
	}
}

// Incoming persists a live message, then applies the arrival policy: read it
// at once if the user is looking at the conversation, otherwise maybe notify.
// A message already stored (same remote id) changes nothing.
func (in *Ingest) Incoming(accountID, conversationRemoteID string, message domain.Message) {
	conv, stored, before, ok := in.persist(accountID, conversationRemoteID, message)
	if !ok {
		return
	}
	arrival := in.session.policyInput(conv, stored)
	if policy.MarkReadOnArrival(arrival) {
		_, _ = in.repo.MarkRead(conv.ID)
	}
	in.publishArrival(conv.ID, stored, before)
	if policy.ShouldNotify(arrival) && in.notifier != nil {
		in.notifier.Notify(policy.Notification(arrival))
	}
}

// History persists an initial or backfilled message without the live
// arrival policy: no notification and no read-on-arrival.
func (in *Ingest) History(accountID, conversationRemoteID string, message domain.Message) {
	if conv, stored, before, ok := in.persist(accountID, conversationRemoteID, message); ok {
		in.publishArrival(conv.ID, stored, before)
	}
}

// persist stores a message for a known conversation. ok is false when the
// conversation is unknown, storage fails, or the message is a duplicate.
// before is the unread total prior to the insert.
func (in *Ingest) persist(accountID, remoteID string, m domain.Message) (domain.Conversation, domain.Message, int, bool) {
	conv, err := in.repo.ConversationByRemote(accountID, remoteID)
	if err != nil {
		return conv, m, 0, false
	}
	m.ConversationID = conv.ID
	if m.Created == 0 {
		m.Created = in.clock.Now().UnixMilli()
	}
	before := in.pub.unreadTotal()
	stored, inserted, err := in.repo.AddMessage(m)
	return conv, stored, before, err == nil && inserted
}

func (in *Ingest) publishArrival(conversationID string, m domain.Message, before int) {
	in.pub.send("message.added", m)
	if _, err := in.pub.conversation(conversationID); err == nil {
		in.pub.unreadChanged(before)
	}
}

// OutgoingStatus records the service's id for a sent message and publishes
// delivery progress that domain.StatusAdvances allows.
func (in *Ingest) OutgoingStatus(localMessageID, remoteID, status string) {
	if remoteID != "" {
		_ = in.repo.SetMessageRemoteID(localMessageID, remoteID)
	}
	if message, changed, err := in.repo.UpdateMessageStatus(localMessageID, status); err == nil && changed {
		in.pub.send("message.updated", message)
	}
}

// Typing publishes a typing indicator for a known conversation.
func (in *Ingest) Typing(accountID, conversationRemoteID, name string, active bool) {
	if conv, err := in.repo.ConversationByRemote(accountID, conversationRemoteID); err == nil {
		in.pub.send("typing", typingEvent{ConversationID: conv.ID, Name: name, Active: active})
	}
}
