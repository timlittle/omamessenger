package whatsapp

// The handlers in live.go are already covered against connectortest.Sink
// in live_test.go; these tests drive the same handlers through a real
// app.Ingest over a real SQLite store instead (see realHandlerFixture),
// so a message that the sink would record but the store would actually
// refuse - the way the Signal titling bug let a message vanish - is
// caught here. Split out of live_test.go to stay under its own
// file-length limit.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestHandleMessage_StoresAMessageFromAnUnnamedSender drives a message
// from a sender with no resolvable name at all - no saved contact, no
// push name - so this proves the chat still gets a non-empty title and
// the message really lands in the store: a chat EnsureConversation
// refuses for an empty title is where the Signal titling bug silently
// dropped every later message for it.
func TestHandleMessage_StoresAMessageFromAnUnnamedSender(t *testing.T) {
	t.Parallel()

	c, dev, ingest, media := realHandlerFixture(t)

	info := liveInfo()
	info.PushName = ""
	c.handleMessage(t.Context(), ingest, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("hi")}})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "15551234567@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title == "" {
		t.Error("conversation title is empty, want a non-empty fallback title")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored despite the unnamed sender", messages, err)
	}
}

// TestHandleMessage_StoresAMessageFromAStranger drives a message from a
// sender who is not a saved contact but whose push name WhatsApp still
// carries (an ordinary unsaved number), confirming the chat is titled
// from that push name and the message is actually stored.
func TestHandleMessage_StoresAMessageFromAStranger(t *testing.T) {
	t.Parallel()

	c, dev, ingest, media := realHandlerFixture(t)

	info := liveInfo()
	info.PushName = "Alex"
	c.handleMessage(t.Context(), ingest, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("hi")}})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "15551234567@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title != "Alex" {
		t.Errorf("conversation title = %q, want the stranger's own push name %q", stored.Title, "Alex")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored", messages, err)
	}
}

// TestHandleMessage_StoresAnOutgoingMessageSyncedFromAnotherDevice
// drives a message WhatsApp reports as sent from this account's own
// linked phone, for a chat already known from an earlier incoming
// message: this path reports through sink.History rather than
// sink.Incoming, a different route through Ingest.save, so it earns its
// own check that the message actually lands in the store.
func TestHandleMessage_StoresAnOutgoingMessageSyncedFromAnotherDevice(t *testing.T) {
	t.Parallel()

	c, dev, ingest, media := realHandlerFixture(t)

	c.handleMessage(t.Context(), ingest, dev, media, &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}})

	outgoing := liveInfo()
	outgoing.IsFromMe, outgoing.ID, outgoing.PushName = true, "M2", ""
	c.handleMessage(t.Context(), ingest, dev, media, &events.Message{
		Info: outgoing, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")},
	})

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "15551234567@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages = %+v, %v; want both messages stored", messages, err)
	}

	var outgoingStored bool
	for _, m := range messages {
		if m.RemoteID == "M2" {
			outgoingStored = m.Outgoing
		}
	}
	if !outgoingStored {
		t.Error("the synced outgoing message was not stored as outgoing")
	}
}

// TestHandleMessage_StoresAGroupMessageWhenGroupInfoLookupFails drives a
// message in a group this connector has never seen, whose group-info
// lookup fails (an unreachable or since-removed group): resolveGroupName
// falls back to the generic "Group" title rather than leaving the chat
// untitled, and this confirms that placeholder is non-empty enough for
// the real store to accept the chat and keep its message, rather than
// refusing it the way the Signal titling bug refused an empty one.
func TestHandleMessage_StoresAGroupMessageWhenGroupInfoLookupFails(t *testing.T) {
	t.Parallel()

	c, dev, ingest, media := realHandlerFixture(t)
	dev.groupErr = errors.New("group info unavailable")

	group := types.NewJID("12345-1600000000", types.GroupServer)
	e := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: liveInfo().Sender, IsGroup: true}, ID: "M2", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi all")},
	}
	c.handleMessage(t.Context(), ingest, dev, media, e)

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "12345-1600000000@g.us")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title != "Group" {
		t.Errorf("conversation title = %q, want the generic fallback %q", stored.Title, "Group")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the message stored despite the failed lookup", messages, err)
	}
}

// TestReportContent_BeforeConversationReported_DropsAndLogs checks that
// a message reported through reportContent before its conversation
// exists in the store - never reachable through handleContent's own
// ensureChat precondition, but reachable if a future caller skips it,
// the way this test does on purpose - is dropped by the real store
// rather than stored unreachably, and that the drop is logged rather
// than silent, without naming the conversation's own remote id (see
// privacy.md).
func TestReportContent_BeforeConversationReported_DropsAndLogs(t *testing.T) {
	t.Parallel()

	c, _, ingest, _ := realHandlerFixture(t)

	e := &events.Message{Info: liveInfo()}
	m := domain.Message{RemoteID: "M1", Text: "hi", Created: 1}
	c.reportContent(t.Context(), ingest, e, "15551234567@s.whatsapp.net", m)

	if _, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "15551234567@s.whatsapp.net"); err == nil {
		t.Fatal("ConversationByRemote() succeeded for a conversation never reported")
	}

	lines := ingest.logger.take()
	if len(lines) != 1 {
		t.Fatalf("logged lines = %v, want exactly one drop logged", lines)
	}
	if strings.Contains(lines[0], "15551234567") {
		t.Errorf("logged line = %q, want it never to name the conversation's own remote id", lines[0])
	}
}
