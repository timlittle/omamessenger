package whatsapp

// The handlers in live.go are unexported, so these tests call them
// directly over a fake device, the way history_test.go drives
// handleHistorySync.

import (
	"strconv"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

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

	c, dev, sink, media := handlerMediaFixture(t)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the new conversation reported", sink.Lines())
	}
	if !sink.Has("incoming 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message reported as incoming", sink.Lines())
	}
}

func TestHandleMessage_ReportsTheOfficialWhatsAppAccountAsAConversation(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	info := liveInfo()
	info.Chat, info.Sender, info.ID, info.PushName = types.PSAJID, types.PSAJID, "M1", ""
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("your security code changed")}})

	if !sink.Has("conversation 0@s.whatsapp.net WhatsApp") {
		t.Errorf("events = %q, want the \"0\" system account shown as a conversation titled WhatsApp", sink.Lines())
	}
	if !sink.Has("incoming 0@s.whatsapp.net M1") {
		t.Errorf("events = %q, want its message reported as incoming", sink.Lines())
	}
}

func TestHandleMessage_SavesTheMessageKeyMarkReadLaterNeeds(t *testing.T) {
	t.Parallel()

	dev, sink, c, media := connectedMediaFixture(t)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}})

	conv := domain.Conversation{RemoteID: "15551234567@s.whatsapp.net", Unread: 1}
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 1 || len(dev.markReadCalls[0].ids) != 1 || string(dev.markReadCalls[0].ids[0]) != "M1" {
		t.Errorf("markRead calls = %+v, want one call noting M1 as read", dev.markReadCalls)
	}
}

func TestHandleMessage_NeverNotesOurOwnMessageFromAnotherDeviceForMarkRead(t *testing.T) {
	t.Parallel()

	dev, sink, c, media := connectedMediaFixture(t)

	info := liveInfo()
	info.IsFromMe = true
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")}})

	// Unread is set as if the service still thought one message was
	// unread here, so this proves the fromMe message's own saved key is
	// excluded by unreadMessageKeys, not merely that nothing was unread
	// to begin with.
	conv := domain.Conversation{RemoteID: "15551234567@s.whatsapp.net", Unread: 1}
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 0 {
		t.Errorf("markRead calls = %+v, want none for a message we sent ourselves", dev.markReadCalls)
	}
}

func TestHandleMessage_ReportsOurOwnMessageFromAnotherDeviceAsHistory(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	info := liveInfo()
	info.IsFromMe = true
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")}})

	if !sink.Has("history 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message reported as history", sink.Lines())
	}
	if sink.Has("conversation 15551234567@s.whatsapp.net WhatsApp user") {
		t.Error("an outgoing message from another device overwrote the conversation's title")
	}
}

// TestHandleMessage_CreatesAChatFromAnOutgoingMessageWhenNoIncomingCameFirst
// covers a message silently vanishing: Ingest.save (backend/internal/app)
// refuses a message for a conversation it has never been told exists, so
// an outgoing-from-another-device message must still ensure its chat the
// first time this connector run sees it, exactly as an incoming one
// would, or a reply typed on the phone to a chat this run has not reached
// yet (a business chat replied to right after a reconnect, say) is
// reported as history and then dropped with nowhere to land.
func TestHandleMessage_CreatesAChatFromAnOutgoingMessageWhenNoIncomingCameFirst(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	info := liveInfo()
	info.IsFromMe, info.PushName = true, ""
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("replied from my phone before this chat was ever seen")}})

	if !sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the chat created so the reply has a conversation to be stored under", sink.Lines())
	}
	if !sink.Has("history 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message still reported as history", sink.Lines())
	}
}

// TestHandleMessage_NeverTitlesAChatWithTheAccountsOwnName confirms a
// chat first seen through an outgoing message is not named after its
// sender: that is this account's own push name, not the other person's.
func TestHandleMessage_NeverTitlesAChatWithTheAccountsOwnName(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	info := liveInfo()
	info.IsFromMe, info.PushName = true, "Me Myself"
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("hi")}})

	if !sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the chat titled from its own fallback, not this account's name", sink.Lines())
	}
}

// TestHandleMessage_NeverRecreatesAnAlreadyKnownChatFromAnOutgoingMessage
// confirms the fix above only covers a chat's first sighting this run:
// once incoming traffic has already reported it, a later outgoing echo
// from another device still skips ensureChat, so it can never overwrite
// a good title with the generic one an outgoing message's own info
// carries.
func TestHandleMessage_NeverRecreatesAnAlreadyKnownChatFromAnOutgoingMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}})
	sink.Take()

	outgoing := liveInfo()
	outgoing.IsFromMe, outgoing.ID, outgoing.PushName = true, "M2", ""
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: outgoing, Message: &waE2E.Message{Conversation: strPtr("my reply")},
	})

	if sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the already-known chat's good title left alone", sink.Lines())
	}
}

func TestHandleMessage_ResolvesAGroupChatItHasNotSeenBefore(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.groupNames = map[string]string{"12345-1600000000@g.us": "Climbing Crew"}
	dev.groupMembers = map[string]int{"12345-1600000000@g.us": 5}

	group := types.NewJID("12345-1600000000", types.GroupServer)
	e := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: liveInfo().Sender, IsGroup: true}, ID: "M2", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi all")},
	}
	c.handleMessage(t.Context(), sink, dev, media, e)

	if !sink.Has("conversation 12345-1600000000@g.us Climbing Crew") {
		t.Errorf("events = %q, want the group's resolved name", sink.Lines())
	}
	if conv, ok := sink.ConversationFor("12345-1600000000@g.us"); !ok || conv.Members != 5 {
		t.Errorf("conversation = %+v, ok=%t, want 5 members resolved alongside the name", conv, ok)
	}
}

func TestHandleMessage_NamesAGroupSenderFromTheirResolvedContact(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.groupNames = map[string]string{"12345-1600000000@g.us": "Climbing Crew"}
	dev.contactNames = map[string]string{"987654@lid": "Priya Nair"}

	group := types.NewJID("12345-1600000000", types.GroupServer)
	sender := types.NewJID("987654", types.HiddenUserServer)
	e := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: sender, IsGroup: true}, ID: "M2", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi all")},
	}
	c.handleMessage(t.Context(), sink, dev, media, e)

	messages := sink.LiveMessages()["12345-1600000000@g.us"]
	if len(messages) != 1 || messages[0].SenderName != "Priya Nair" {
		t.Errorf("live messages = %+v, want the sender named from their resolved contact", messages)
	}
}

func TestHandleMessage_TitlesALIDDirectChatFromItsContact(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.contactNames = map[string]string{"987654@lid": "Priya Nair"}

	chat := types.NewJID("987654", types.HiddenUserServer)
	c.handleMessage(t.Context(), sink, dev, media, directMessage(chat, "M2", "a stray push name"))

	if !sink.Has("conversation 987654@lid Priya Nair") {
		t.Errorf("events = %q, want the LID chat titled from its resolved contact, not the push name", sink.Lines())
	}
}

// directMessage is an incoming message in a direct chat addressed by
// chat (its own sender too, as a direct chat's partner always is).
func directMessage(chat types.JID, id, pushName string) *events.Message {
	return &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: id, PushName: pushName, Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi")},
	}
}

// TestHandleMessage_RemembersANameLearnedUnderTheOtherAddressForm
// covers the actual bug this fix closes: a name learned while a chat
// was addressed by its phone JID must still title it once WhatsApp
// reports the exact same chat addressed by its mapped LID instead,
// rather than losing it to the weaker fallback resolveDirectTitle's
// own phoneFormOf tests already cover on its own (see history_test.go).
func TestHandleMessage_RemembersANameLearnedUnderTheOtherAddressForm(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	c.handleMessage(t.Context(), sink, dev, media, directMessage(phone, "M1", "Acme Shop"))
	sink.Take()
	c.handleMessage(t.Context(), sink, dev, media, directMessage(lid, "M2", ""))

	if !sink.Has("conversation 15551234567@s.whatsapp.net Acme Shop") {
		t.Errorf("events = %q, want the name learned under the phone JID kept", sink.Lines())
	}
}

func TestHandleMessage_SkipsAProtocolNoticeWithNoContent(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: liveInfo(), Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum()}},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want a protocol notice with no content to report nothing at all", sink.Lines())
	}
}

func TestHandleMessage_SkipsAMessageInASystemChat(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID}, ID: "M9", Timestamp: time.Unix(1, 0)}, Message: &waE2E.Message{Conversation: strPtr("someone's status")},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a system JID such as the status broadcast", sink.Lines())
	}
}

func TestHandleMessage_CreatesAndFillsTheSelfChatFromAnOutgoingMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	self := types.NewJID("15551234567", types.DefaultUserServer)
	dev.selfJID = self

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: self, Sender: self, IsFromMe: true}, ID: "M1", Timestamp: time.Unix(1, 0)}, Message: &waE2E.Message{Conversation: strPtr("note to self")},
	})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Message yourself") {
		t.Errorf("events = %q, want the self-chat created and titled \"Message yourself\"", sink.Lines())
	}
	if !sink.Has("history 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the self-sent message reported as history, never as unread", sink.Lines())
	}
}

func TestHandleMessage_CollapsesTheSelfChatsLIDAndPhoneJIDIntoOneConversation(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	phone, lid := types.NewJID("15551234567", types.DefaultUserServer), types.NewJID("111222", types.HiddenUserServer)
	dev.selfJID, dev.selfLID = phone, lid

	// The user's own message, sent and addressed by phone JID.
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: phone, Sender: phone, IsFromMe: true}, ID: "M1", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("note to self")},
	})

	// A bot's reply, posted from another of this account's own linked
	// devices, addressed by the account's LID instead.
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid, IsFromMe: true}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("reply from the bot")},
	})

	if !sink.Has("history 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the phone-addressed message filed under the phone JID", sink.Lines())
	}
	if !sink.Has("history 15551234567@s.whatsapp.net M2") {
		t.Errorf("events = %q, want the LID-addressed message filed under the same phone JID, not a second conversation", sink.Lines())
	}
	if sink.Has("conversation 111222@lid Message yourself") {
		t.Errorf("events = %q, want no separate conversation created for the LID form", sink.Lines())
	}
}

func TestHandleMessage_PersistsAMediaReference(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		DirectPath: strPtr("/v/x"), MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3},
		FileLength: u64(1), Mimetype: strPtr("image/jpeg"),
	}}}
	c.handleMessage(t.Context(), sink, dev, media, e)

	if _, ok, err := media.get(t.Context(), "15551234567@s.whatsapp.net", "M1"); err != nil || !ok {
		t.Errorf("media.get = ok=%v err=%v, want the reference saved", ok, err)
	}
}

// TestHandleMessage_ReportsEachReactionChangeAsTheFullTally also covers
// the bug fixed by canonicalizing reactorKey (see personID in
// normalize.go): Nadia reacting once addressed by her LID and again by
// her mapped phone JID must update her one chip, never add a second
// one for what would otherwise look like a different reactor.
func TestHandleMessage_ReportsEachReactionChangeAsTheFullTally(t *testing.T) {
	t.Parallel()

	nadia := types.NewJID("15551234567", types.DefaultUserServer)
	nadiaLID := types.NewJID("987654", types.HiddenUserServer)

	c, dev, sink, media := handlerMediaFixture(t)
	dev.lidPhones = map[string]types.JID{nadiaLID.String(): nadia}

	react := func(sender types.JID, fromMe bool, emoji string) *events.Message {
		return &events.Message{
			Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: nadia, Sender: sender, IsFromMe: fromMe}},
			Message: &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
				Key: &waCommon.MessageKey{ID: strPtr("M1")}, Text: strPtr(emoji),
			}},
		}
	}
	assertTally := func(want int) {
		if line := "reacted 15551234567@s.whatsapp.net M1 " + strconv.Itoa(want); !sink.Has(line) {
			t.Errorf("events = %q, want %q", sink.Lines(), line)
		}
	}

	c.handleMessage(t.Context(), sink, dev, media, react(nadia, false, "👍"))
	assertTally(1)

	// We react too, with a different emoji: now two chips.
	c.handleMessage(t.Context(), sink, dev, media, react(types.JID{}, true, "❤️"))
	assertTally(2)

	// Nadia reacts again, this time addressed by her LID: still two
	// chips, not three, since this must update her existing one (see
	// normalize.go's personID, which fixed this double count).
	c.handleMessage(t.Context(), sink, dev, media, react(nadiaLID, false, "😂"))
	assertTally(2)

	// Nadia clears her reaction, by her phone JID this time; back down
	// to one chip, confirming both of her forms shared one reactor key.
	c.handleMessage(t.Context(), sink, dev, media, react(nadia, false, ""))
	assertTally(1)
}

func TestHandleMessage_RevokeAndEdit(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	revokeMsg := &events.Message{
		Info:    liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, revokeMsg)
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
	c.handleMessage(t.Context(), sink, dev, media, editMsg)
	if !sink.Has("edited 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the edit reported", sink.Lines())
	}
}

func TestHandleChatPresence_NamesTheTyperOnlyInAGroup(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	c.rememberName("15551234567@s.whatsapp.net", "Nadia", nameRankPushName)

	direct := types.NewJID("15551234567", types.DefaultUserServer)
	c.handleChatPresence(t.Context(), sink, dev, nil, &events.ChatPresence{
		MessageSource: types.MessageSource{Chat: direct, Sender: direct}, State: types.ChatPresenceComposing,
	})
	if !sink.Has("typing 15551234567@s.whatsapp.net true") {
		t.Errorf("events = %q, want typing with no name in a direct chat", sink.Lines())
	}

	group := types.NewJID("12345-1600000000", types.GroupServer)
	c.handleChatPresence(t.Context(), sink, dev, nil, &events.ChatPresence{
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

			c, dev, sink := handlerFixture(t)
			c.handleReceipt(t.Context(), sink, dev, nil, tt.evt)

			got := sink.Has("unread 15551234567@s.whatsapp.net 0")
			if got != tt.want {
				t.Errorf("handled = %v, want %v; events = %q", got, tt.want, sink.Lines())
			}
		})
	}
}

func TestHandleUndecryptable_ReportsAPlaceholderForAnIncomingMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	c.handleUndecryptable(t.Context(), sink, dev, nil, &events.UndecryptableMessage{Info: liveInfo()})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the chat ensured so the placeholder has somewhere to live", sink.Lines())
	}
	live := sink.LiveMessages()["15551234567@s.whatsapp.net"]
	if len(live) != 1 || live[0].Text != undecryptablePlaceholder {
		t.Errorf("placeholder text = %+v, want %q", live, undecryptablePlaceholder)
	}
}

// TestHandleUndecryptable_CreatesAChatFromAnOutgoingMessageWhenNoIncomingCameFirst
// is handleContent's own fix (see
// TestHandleMessage_CreatesAChatFromAnOutgoingMessageWhenNoIncomingCameFirst)
// for the same gap in the undecryptable path: an own-device message
// that could not be decrypted yet must still ensure its chat the first
// time this run sees it, or its placeholder is reported for a
// conversation Ingest has never been told exists and is silently
// dropped.
func TestHandleUndecryptable_CreatesAChatFromAnOutgoingMessageWhenNoIncomingCameFirst(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)

	info := liveInfo()
	info.IsFromMe, info.PushName = true, ""
	c.handleUndecryptable(t.Context(), sink, dev, nil, &events.UndecryptableMessage{Info: info})

	if !sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the chat created so the placeholder has a conversation to be stored under", sink.Lines())
	}
}

func TestHandleUndecryptable_SkipsASystemChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	c.handleUndecryptable(t.Context(), sink, dev, nil, &events.UndecryptableMessage{
		Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID}, ID: "M9"},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a system JID", sink.Lines())
	}
}

// TestHandleContent_ReplacesAnUndecryptablePlaceholderOnRedelivery covers
// both ways a redelivery can reach handleContent with the same id as a
// placeholder: the original sender's own retry (no UnavailableRequestID),
// or AutomaticMessageRerequestFromPhone's request answered by the
// primary phone instead, once the sender never answers (UnavailableRequestID
// set; see device.go).
func TestHandleContent_ReplacesAnUndecryptablePlaceholderOnRedelivery(t *testing.T) {
	t.Parallel()

	for _, reqID := range []string{"", "REQ1"} {
		c, dev, sink, media := handlerMediaFixture(t)
		c.handleUndecryptable(t.Context(), sink, dev, media, &events.UndecryptableMessage{Info: liveInfo()})
		msg := &waE2E.Message{Conversation: strPtr("hi")}
		c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: liveInfo(), Message: msg, UnavailableRequestID: reqID})
		if !sink.Has("edited 15551234567@s.whatsapp.net M1") {
			t.Errorf("events = %q, want the redelivered message to replace the placeholder", sink.Lines())
		}
		if live := sink.LiveMessages()["15551234567@s.whatsapp.net"]; len(live) != 1 {
			t.Errorf("live messages = %+v, want only the placeholder's single incoming report", live)
		}
	}
}
