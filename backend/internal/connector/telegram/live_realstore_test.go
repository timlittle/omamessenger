package telegram

// The handlers in live.go are already covered against connectortest.Sink
// in live_test.go; these tests drive the same handlers through a real
// app.Ingest over a real SQLite store instead (see newRealIngest), so a
// message that the sink would record but the store would actually
// refuse - the way the Signal titling bug let a message vanish - is
// caught here. Split out of live_test.go to stay under its own
// file-length limit.

import (
	"testing"

	"github.com/gotd/td/tg"
)

// TestNewMessage_StoresAMessageFromAnUnresolvedSender drives a message
// whose sender is in neither the update's own entities nor this
// connector's cache (a short message, or an UpdateNewMessage missing
// its sender): placeholderConversation falls back to the generic
// "Telegram user" title, and this proves that placeholder is non-empty
// enough for the real store to accept the chat and keep its message,
// rather than refusing it the way the Signal titling bug refused an
// empty one.
func TestNewMessage_StoresAMessageFromAnUnresolvedSender(t *testing.T) {
	t.Parallel()

	c := realConnector()
	ingest := newRealIngest(t)

	msg := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}
	c.newMessage(t.Context(), ingest, msg, tg.Entities{})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "user:42:0")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title == "" {
		t.Error("conversation title is empty, want a non-empty fallback title")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored despite the unresolved sender", messages, err)
	}
}

// TestNewMessage_StoresAMessageFromAStranger drives a message from a
// sender whose user entity Telegram did include, but who is not a
// saved contact (an unsaved username, the ordinary case for a direct
// message from someone outside the account's contacts): ownName falls
// back to "@username" rather than a first and last name, and this
// confirms the chat is titled from it and the message is actually
// stored.
func TestNewMessage_StoresAMessageFromAStranger(t *testing.T) {
	t.Parallel()

	c := realConnector()
	ingest := newRealIngest(t)
	stranger := &tg.User{ID: 42, AccessHash: 99, Username: "alexstranger"}

	msg := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}
	c.newMessage(t.Context(), ingest, msg, tg.Entities{Users: map[int64]*tg.User{42: stranger}})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "user:42:99")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title != "@alexstranger" {
		t.Errorf("conversation title = %q, want the stranger's own username %q", stored.Title, "@alexstranger")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored", messages, err)
	}
}

// TestNewMessage_StoresAnOutgoingMessageSyncedFromAnotherDevice drives
// a message Telegram reports as Out (sent from this account, whether
// from this helper or another of its logged-in devices): newMessage
// reports it through sink.History rather than sink.Incoming, a
// different route through Ingest.save, so it earns its own check that
// the message actually lands in the store.
func TestNewMessage_StoresAnOutgoingMessageSyncedFromAnotherDevice(t *testing.T) {
	t.Parallel()

	c := realConnector()
	ingest := newRealIngest(t)

	msg := &tg.Message{ID: 7, Out: true, PeerID: &tg.PeerUser{UserID: 42}, Message: "sent from another device"}
	c.newMessage(t.Context(), ingest, msg, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "user:42:99")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 || !messages[0].Outgoing {
		t.Fatalf("messages = %+v, %v; want one outgoing message stored", messages, err)
	}
}

// TestNewMessage_StoresAGroupMessageWhenThePeerIsUnresolved drives a
// message in a basic group this connector has never seen, whose peer
// is not among the update's own entities (an info lookup Telegram
// never answered for it): placeholderConversation falls back to the
// generic "Telegram group" title, confirming that placeholder lets the
// real store accept the chat and keep its message, rather than
// refusing it the way the Signal titling bug refused an empty one.
func TestNewMessage_StoresAGroupMessageWhenThePeerIsUnresolved(t *testing.T) {
	t.Parallel()

	c := realConnector()
	ingest := newRealIngest(t)

	msg := &tg.Message{ID: 4, PeerID: &tg.PeerChat{ChatID: 7}, Message: "hi all"}
	c.newMessage(t.Context(), ingest, msg, tg.Entities{})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "chat:7")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title != unknownGroupName {
		t.Errorf("conversation title = %q, want the generic fallback %q", stored.Title, unknownGroupName)
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored despite the unresolved peer", messages, err)
	}
}
