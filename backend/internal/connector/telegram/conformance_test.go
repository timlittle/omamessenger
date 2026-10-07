package telegram

import (
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// This file runs the shared connector conformance checks against Telegram,
// as far as they can run offline against the fake Telegram API. Run needs
// a live connection to gotd/td's servers to sign in and sync, so
// CheckLifecycle cannot run here; it is exercised by the fake connector
// instead.
//
// newMessage and message are unexported, and the recording Sink has no
// public way to drive them without a live update, so these tests reach
// into the package rather than through Connector's exported methods.

// TestConformance_SendProgress runs the shared send-progress check against
// a message that Telegram reports sent and then read.
func TestConformance_SendProgress(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesSendMessageRequest{}, &tg.UpdateShortSentMessage{ID: 77})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	c.learn(chatWithNadia.RemoteID)
	if err := c.Send(t.Context(), chatWithNadia, domain.Message{ID: "progress", Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	c.readUpTo(t.Context(), &sink, "user:42", 77)

	connectortest.CheckSendProgress(t, &sink, "progress")
}

// TestConformance_Incoming runs the shared incoming-fields check against a
// message normalized from a live update.
func TestConformance_Incoming(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	msg := &tg.Message{ID: 30, Date: 1_800_000_000, PeerID: &tg.PeerUser{UserID: 42}, Message: "checking in"}
	c.newMessage(t.Context(), &sink, msg, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	live := sink.LiveMessages()["user:42:99"]
	if len(live) != 1 {
		t.Fatalf("live messages = %d, want 1", len(live))
	}

	connectortest.CheckIncoming(t, live[0])
}

// TestConformance_DuplicateUpdate runs the shared duplicate-delivery check
// against the same update delivered twice, as Telegram's update dispatcher
// can after a gap is recovered.
func TestConformance_DuplicateUpdate(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	entities := tg.Entities{Users: map[int64]*tg.User{42: nadia()}}
	msg := &tg.Message{ID: 31, PeerID: &tg.PeerUser{UserID: 42}, Message: "still there?"}

	c.newMessage(t.Context(), &sink, msg, entities)
	c.newMessage(t.Context(), &sink, msg, entities)

	live := sink.LiveMessages()["user:42:99"]
	if len(live) != 2 {
		t.Fatalf("live messages = %d, want 2", len(live))
	}

	connectortest.CheckDuplicates(t, live[0], live[1])
}

// TestConformance_DuplicateHistory runs the shared duplicate-delivery
// check against a history page fetched twice, as a reconnect's resync
// does for a conversation whose history has not changed.
func TestConformance_DuplicateHistory(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 39, PeerID: &tg.PeerUser{UserID: 42}, Message: "older"}},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)

	for range 2 {
		if _, err := c.LoadOlder(t.Context(), chatWithNadia, "40", 30); err != nil {
			t.Fatal(err)
		}
	}

	history := sink.Messages()["user:42:99"]
	if len(history) != 2 {
		t.Fatalf("history messages = %d, want 2", len(history))
	}

	connectortest.CheckDuplicates(t, history[0], history[1])
}
