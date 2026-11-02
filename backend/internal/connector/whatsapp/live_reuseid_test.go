package whatsapp

// live_reuseid_test.go covers live.go's handleContent refusing to
// report anything to the sink for a message whose id is already known
// under a different sender (see keys.go's reusesAnotherSendersID):
// the fix for an incomplete version of an earlier security fix, kept
// separate from live_test.go's own happy-path and undecryptable-replay
// tests to stay within this codebase's file-length limit.

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// TestHandleContent_DropsALiveMessageThatReusesAnotherSendersID covers
// the fix for an incomplete version of an earlier security fix:
// reusesAnotherSendersID (see keys.go) already stopped Mallory's reused
// id from overwriting the message_keys and media_ref rows Alice's
// message first saved, but handleContent still went on to report
// Mallory's own message to the sink as if it were legitimate, which
// would have attached her text, media or reply to whatever the UI still
// shows as Alice's message. Nothing about a rejected reuse may reach
// the sink at all.
func TestHandleContent_DropsALiveMessageThatReusesAnotherSendersID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	alice := types.NewJID("15551111111", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: alice}, ID: "M0", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi, it's Alice")},
	})
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("forged")},
	})

	live := sink.LiveMessages()[remoteID(chat)]
	if len(live) != 1 || live[0].Text != "hi, it's Alice" {
		t.Errorf("live messages = %+v, want only Alice's own message, nothing from Mallory reusing its id", live)
	}
}

// TestHandleContent_DropsALiveMessageThatReusesOurOwnMessagesID covers
// the same fix for a message this account sent itself, reported from
// another of its devices: an incoming sender reusing that id must never
// have their own content reported as if it belonged to our own message.
func TestHandleContent_DropsALiveMessageThatReusesOurOwnMessagesID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, IsFromMe: true}, ID: "M0", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("sent from my phone")},
	})
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("forged")},
	})

	if live := sink.LiveMessages()[remoteID(chat)]; len(live) != 0 {
		t.Errorf("live messages = %+v, want nothing reported for Mallory reusing our own message's id", live)
	}

	history := sink.Messages()[remoteID(chat)]
	if len(history) != 1 || history[0].Text != "sent from my phone" {
		t.Errorf("history messages = %+v, want only our own message, reported once", history)
	}
}
