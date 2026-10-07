package whatsapp

// The handlers in live.go are unexported, so these tests call them
// directly over a fake device, the way history_test.go drives
// handleHistorySync.

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// liveInfo is an incoming message in a direct chat from Nadia.
func liveInfo() types.MessageInfo {
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:   types.NewJID("15551234567", types.DefaultUserServer),
			Sender: types.NewJID("15551234567", types.DefaultUserServer),
		},
		ID: "M1", PushName: "Nadia", Timestamp: time.Unix(1, 0),
	}
}

func TestHandleMessage_ReportsIncomingContentAndItsConversation(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the new conversation reported", sink.Lines())
	}
	if !sink.Has("incoming 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message reported as incoming", sink.Lines())
	}
}

func TestHandleMessage_NotesIncomingContentForMarkRead(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media := newTestMediaStore(t)

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	conv := domain.Conversation{RemoteID: "15551234567@s.whatsapp.net"}
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 1 || len(dev.markReadCalls[0].ids) != 1 || string(dev.markReadCalls[0].ids[0]) != "M1" {
		t.Errorf("markRead calls = %+v, want one call noting M1 as read", dev.markReadCalls)
	}
}

func TestHandleMessage_NeverNotesOurOwnMessageFromAnotherDeviceForMarkRead(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	media := newTestMediaStore(t)

	info := liveInfo()
	info.IsFromMe = true
	e := &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	conv := domain.Conversation{RemoteID: "15551234567@s.whatsapp.net"}
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 0 {
		t.Errorf("markRead calls = %+v, want none for a message we sent ourselves", dev.markReadCalls)
	}
}

func TestHandleMessage_ReportsOurOwnMessageFromAnotherDeviceAsHistory(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	info := liveInfo()
	info.IsFromMe = true
	e := &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	if !sink.Has("history 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message reported as history", sink.Lines())
	}
	if sink.Has("conversation 15551234567@s.whatsapp.net WhatsApp user") {
		t.Error("an outgoing message from another device overwrote the conversation's title")
	}
}

func TestHandleMessage_ResolvesAGroupChatItHasNotSeenBefore(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	dev.groupNames = map[string]string{"12345-1600000000@g.us": "Climbing Crew"}
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	group := types.NewJID("12345-1600000000", types.GroupServer)
	e := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: liveInfo().Sender, IsGroup: true}, ID: "M2", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi all")},
	}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 12345-1600000000@g.us Climbing Crew") {
		t.Errorf("events = %q, want the group's resolved name", sink.Lines())
	}
}

func TestHandleMessage_PersistsAMediaReference(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		DirectPath: strPtr("/v/x"), MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3},
		FileLength: u64(1), Mimetype: strPtr("image/jpeg"),
	}}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	if _, ok, err := media.get(t.Context(), "15551234567@s.whatsapp.net", "M1"); err != nil || !ok {
		t.Errorf("media.get = ok=%v err=%v, want the reference saved", ok, err)
	}
}

func TestHandleMessage_ReportsEachReactionChangeAsTheFullTally(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	chat := types.NewJID("15551234567", types.DefaultUserServer)
	react := func(sender types.JID, fromMe bool, emoji string) *events.Message {
		return &events.Message{
			Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: fromMe}},
			Message: &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
				Key: &waCommon.MessageKey{ID: strPtr("M1")}, Text: strPtr(emoji),
			}},
		}
	}

	nadia := types.NewJID("15551234567", types.DefaultUserServer)
	c.handleMessage(t.Context(), &sink, dev, media, react(nadia, false, "👍"))
	if !sink.Has("reacted 15551234567@s.whatsapp.net M1 1") {
		t.Errorf("events = %q, want one reaction chip", sink.Lines())
	}

	// We react too, with a different emoji: now two chips.
	c.handleMessage(t.Context(), &sink, dev, media, react(types.JID{}, true, "❤️"))
	if !sink.Has("reacted 15551234567@s.whatsapp.net M1 2") {
		t.Errorf("events = %q, want two reaction chips", sink.Lines())
	}

	// Nadia clears her reaction; back down to one chip.
	c.handleMessage(t.Context(), &sink, dev, media, react(nadia, false, ""))
	if !sink.Has("reacted 15551234567@s.whatsapp.net M1 1") {
		t.Errorf("events = %q, want one chip left after a reaction is cleared", sink.Lines())
	}
}

func TestHandleMessage_RevokeAndEdit(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	revokeMsg := &events.Message{
		Info:    liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), &sink, dev, media, revokeMsg)
	if !sink.Has("deleted 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the revoked message reported deleted", sink.Lines())
	}

	editMsg := &events.Message{
		Info: liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")},
			EditedMessage: &waE2E.Message{Conversation: strPtr("corrected")},
		}},
	}
	c.handleMessage(t.Context(), &sink, dev, media, editMsg)
	if !sink.Has("edited 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the edit reported", sink.Lines())
	}
}

func TestHandleChatPresence_NamesTheTyperOnlyInAGroup(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	c.setName("15551234567@s.whatsapp.net", "Nadia")
	var sink connectortest.Sink

	direct := types.NewJID("15551234567", types.DefaultUserServer)
	c.handleChatPresence(t.Context(), &sink, &events.ChatPresence{
		MessageSource: types.MessageSource{Chat: direct, Sender: direct}, State: types.ChatPresenceComposing,
	})
	if !sink.Has("typing 15551234567@s.whatsapp.net true") {
		t.Errorf("events = %q, want typing with no name in a direct chat", sink.Lines())
	}

	group := types.NewJID("12345-1600000000", types.GroupServer)
	c.handleChatPresence(t.Context(), &sink, &events.ChatPresence{
		MessageSource: types.MessageSource{Chat: group, Sender: direct, IsGroup: true}, State: types.ChatPresencePaused,
	})
	if !sink.Has("typing 12345-1600000000@g.us false") {
		t.Errorf("events = %q, want typing stopped reported", sink.Lines())
	}
}

func TestHandleReceipt_OnlyActsOnOurOwnReadReceipts(t *testing.T) {
	t.Parallel()

	chat := types.NewJID("15551234567", types.DefaultUserServer)

	tests := []struct {
		name string
		evt  *events.Receipt
		want bool
	}{
		{"read from another of our devices", &events.Receipt{MessageSource: types.MessageSource{Chat: chat, IsFromMe: true}, Type: types.ReceiptTypeRead}, true},
		{"read-self from another device", &events.Receipt{MessageSource: types.MessageSource{Chat: chat, IsFromMe: true}, Type: types.ReceiptTypeReadSelf}, true},
		{"delivered receipt about our own message", &events.Receipt{MessageSource: types.MessageSource{Chat: chat, IsFromMe: false}, Type: types.ReceiptTypeDelivered}, false},
		{"read receipt about our own message", &events.Receipt{MessageSource: types.MessageSource{Chat: chat, IsFromMe: false}, Type: types.ReceiptTypeRead}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(domain.Account{ID: "wa"}, t.TempDir())
			var sink connectortest.Sink
			c.handleReceipt(t.Context(), &sink, tt.evt)

			got := sink.Has("unread 15551234567@s.whatsapp.net 0")
			if got != tt.want {
				t.Errorf("handled = %v, want %v; events = %q", got, tt.want, sink.Lines())
			}
		})
	}
}

func TestHandlePinAndArchive_MergeWithTheOtherKnownFlag(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	var sink connectortest.Sink
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handlePin(t.Context(), &sink, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	if !sink.Has("organized 15551234567@s.whatsapp.net true false") {
		t.Errorf("events = %q, want pinned true archived false", sink.Lines())
	}

	c.handleArchive(t.Context(), &sink, &events.Archive{JID: jid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	if !sink.Has("organized 15551234567@s.whatsapp.net true true") {
		t.Errorf("events = %q, want pinned still true, archived now true", sink.Lines())
	}

	c.handlePin(t.Context(), &sink, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(false)}})
	if !sink.Has("organized 15551234567@s.whatsapp.net false true") {
		t.Errorf("events = %q, want pinned false, archived still true", sink.Lines())
	}
}
