package whatsapp

// handleHistorySync is unexported, with no way to drive a real one of
// these blobs without reaching WhatsApp, so these tests call it
// directly, as the Telegram connector's sync tests do for its own
// unexported sync step.

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// historyMsg builds one history sync message with text, FromMe and a
// timestamp of 1.
func historyMsg(id, text string, fromMe bool) *waHistorySync.HistorySyncMsg {
	return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr(id), FromMe: boolPtr(fromMe)},
		Message:          &waE2E.Message{Conversation: strPtr(text)},
		MessageTimestamp: u64(1),
	}}
}

// historySyncEvent wraps convs in the event envelope handleHistorySync
// expects, the way a real history-sync blob arrives with one or more
// conversations.
func historySyncEvent(convs ...*waHistorySync.Conversation) *events.HistorySync {
	return &events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: convs}}
}

func TestHandleHistorySync_ReportsContactsConversationsAndMessages(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	// Pinned and archived are read from whatsmeow's own chat settings
	// store, never from the sync blob itself (see docs/decisions.md),
	// so they are seeded there instead of on the synced conversations
	// below.
	dev.setChatSettings("15551234567@s.whatsapp.net", true, false)
	dev.setChatSettings("12345-1600000000@g.us", false, true)

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Pushnames: []*waHistorySync.Pushname{{ID: strPtr("15551234567@s.whatsapp.net"), Pushname: strPtr("Nadia")}},
		Conversations: []*waHistorySync.Conversation{
			{
				ID: strPtr("15551234567@s.whatsapp.net"), UnreadCount: u32(1),
				Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
			},
			{
				ID: strPtr("12345-1600000000@g.us"), DisplayName: strPtr("Climbing Crew"),
				Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H2", "see you there", false)},
			},
		},
	}}

	c.handleHistorySync(t.Context(), sink, dev, media, e)

	want := []string{
		"contact 15551234567@s.whatsapp.net Nadia",
		"conversation 15551234567@s.whatsapp.net Nadia",
		"organized 15551234567@s.whatsapp.net true false",
		"history 15551234567@s.whatsapp.net H1",
		"unread 15551234567@s.whatsapp.net 1",
		"conversation 12345-1600000000@g.us Climbing Crew",
		"organized 12345-1600000000@g.us false true",
		"history 12345-1600000000@g.us H2",
		"unread 12345-1600000000@g.us 0",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events =\n%q\nwant\n%q", got, want)
	}
}

// groupJIDForNaming is the group resolveGroupName's tests resolve a
// name and member count for, directly, without going through a full
// history sync event.
var groupJIDForNaming = types.NewJID("12345-1600000000", types.GroupServer)

func TestResolveGroupName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		dev           *fakeDevice
		syncedTitle   string
		syncedMembers int
		wantName      string
		wantMembers   int
		wantCalls     int
	}{
		{
			"resolves an unknown group's name and members together",
			&fakeDevice{groupNames: map[string]string{groupJIDForNaming.String(): "Climbing Crew"}, groupMembers: map[string]int{groupJIDForNaming.String(): 12}},
			"", 0, "Climbing Crew", 12, 1,
		},
		{
			"falls back to a generic name when it cannot be resolved",
			&fakeDevice{groupErr: errors.New("unavailable")},
			"", 0, "Group", 0, 1,
		},
		{
			"keeps the sync's own member count without asking WhatsApp",
			&fakeDevice{},
			"Climbing Crew", 2, "Climbing Crew", 2, 0,
		},
		{
			// Scripted differently from the synced title, to prove the
			// fetched name is never used once a name is already known:
			// only its member count matters here, since the sync, as
			// WhatsApp's own history sync often does, named the group
			// but carried no participant list for it at all.
			"resolves member count even when the sync already named the group",
			&fakeDevice{groupNames: map[string]string{groupJIDForNaming.String(): "should never be used"}, groupMembers: map[string]int{groupJIDForNaming.String(): 3}},
			"Tim and Laura", 0, "Tim and Laura", 3, 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(domain.Account{ID: "wa"}, t.TempDir())
			name, members := c.resolveGroupName(t.Context(), tt.dev, groupJIDForNaming, tt.syncedTitle, tt.syncedMembers)

			if name != tt.wantName || members != tt.wantMembers {
				t.Errorf("resolveGroupName = (%q, %d), want (%q, %d)", name, members, tt.wantName, tt.wantMembers)
			}
			if len(tt.dev.groupCalls) != tt.wantCalls {
				t.Errorf("group info requested %d times, want %d", len(tt.dev.groupCalls), tt.wantCalls)
			}
		})
	}
}

func TestResolveGroupName_CachesAfterItsFirstResolve(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := &fakeDevice{groupNames: map[string]string{groupJIDForNaming.String(): "Climbing Crew"}}

	c.resolveGroupName(t.Context(), dev, groupJIDForNaming, "", 0)
	c.resolveGroupName(t.Context(), dev, groupJIDForNaming, "", 0)

	if len(dev.groupCalls) != 1 {
		t.Errorf("group info requested %d times across two resolves, want 1 (cached after the first)", len(dev.groupCalls))
	}
}

func TestHandleHistorySync_NamesAGroupSenderFromAnAlreadyCachedPushName(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	laura := types.NewJID("15559990000", types.DefaultUserServer)
	c.rememberName(remoteID(laura), "Laura", nameRankPushName) // seeded the way syncPushnames does, before this conversation's own messages are processed

	// The message itself carries no push name of its own, which is
	// common for a group's history sync, so naming its sender has to
	// fall back to the cache seeded above rather than the generic
	// "WhatsApp user".
	msg := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1"), FromMe: boolPtr(false), Participant: strPtr(laura.String())},
		Message:          &waE2E.Message{Conversation: strPtr("see you there")},
		MessageTimestamp: u64(1),
	}}
	conv := &waHistorySync.Conversation{
		ID: strPtr("12345-1600000000@g.us"), DisplayName: strPtr("Tim and Laura"),
		Messages: []*waHistorySync.HistorySyncMsg{msg},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	live := sink.Messages()["12345-1600000000@g.us"]
	if len(live) != 1 || live[0].SenderName != "Laura" {
		t.Errorf("messages = %+v, want the sender named from the cached push name, not the generic fallback", live)
	}
}

func TestHandleHistorySync_ResolvesALIDChatToItsContactsSavedName(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.contactNames = map[string]string{"987654@lid": "Priya Nair"}

	conv := &waHistorySync.Conversation{ID: strPtr("987654@lid"), Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 987654@lid Priya Nair") {
		t.Errorf("events = %q, want the LID chat titled with its contact's name", sink.Lines())
	}
}

func TestHandleHistorySync_FallsBackToANeutralLabelForAnUnresolvedLID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	conv := &waHistorySync.Conversation{ID: strPtr("987654@lid"), Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 987654@lid Unknown contact") {
		t.Errorf("events = %q, want a neutral label rather than the hidden id formatted as a phone number", sink.Lines())
	}
}

// TestHandleHistorySync_FallsBackToThePhoneNumberForALIDChatWithAKnownMapping
// covers a chat history sync still only addresses by its LID even
// though whatsmeow has since learned the mapping to a phone JID: the
// fallback title must read it from the mapped phone JID, never format
// the hidden id itself as if it were a phone number.
func TestHandleHistorySync_FallsBackToThePhoneNumberForALIDChatWithAKnownMapping(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.lidPhones = map[string]types.JID{"987654@lid": types.NewJID("15551234567", types.DefaultUserServer)}

	conv := &waHistorySync.Conversation{ID: strPtr("987654@lid"), Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 987654@lid +15551234567") {
		t.Errorf("events = %q, want the fallback title read from the mapped phone JID", sink.Lines())
	}
}

// TestHandleHistorySync_TitlesALIDChatFromAMessagesPushName covers a
// business or non-contact account whose chat carries no name of its
// own in the sync's Conversation entry, but whose individual messages
// each carry a push name: that name must title the chat, rather than
// the neutral "Unknown contact" label.
func TestHandleHistorySync_TitlesALIDChatFromAMessagesPushName(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	msg := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1"), FromMe: boolPtr(false)},
		Message:          &waE2E.Message{Conversation: strPtr("hi")},
		MessageTimestamp: u64(1),
		PushName:         strPtr("Priya's Boutique"),
	}}
	conv := &waHistorySync.Conversation{ID: strPtr("987654@lid"), Messages: []*waHistorySync.HistorySyncMsg{msg}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 987654@lid Priya's Boutique") {
		t.Errorf("events = %q, want the chat titled from a synced message's own push name", sink.Lines())
	}
}

// TestHandleHistorySync_CreatesEveryListedConversation confirms every
// chat the phone lists in history sync appears, even one whose synced
// messages carry nothing to show: it is still a chat on the phone, and
// a later message needs it to exist.
func TestHandleHistorySync_CreatesEveryListedConversation(t *testing.T) {
	t.Parallel()

	reaction := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1")},
		Message:          &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommon.MessageKey{ID: strPtr("H0")}, Text: strPtr("👍")}},
		MessageTimestamp: u64(1),
	}}

	tests := []struct {
		name string
		conv *waHistorySync.Conversation
		want string
	}{
		{"no messages at all", &waHistorySync.Conversation{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia")}, "conversation 15551234567@s.whatsapp.net Nadia"},
		{"only a reaction", &waHistorySync.Conversation{ID: strPtr("987654@lid"), Messages: []*waHistorySync.HistorySyncMsg{reaction}}, "conversation 987654@lid Unknown contact"},
		{"WhatsApp's own system chat", &waHistorySync.Conversation{ID: strPtr("0@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{reaction}}, "conversation 0@s.whatsapp.net WhatsApp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, dev, sink, media := handlerMediaFixture(t)
			c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(tt.conv))

			if !sink.Has(tt.want) {
				t.Errorf("events = %q, want %q", sink.Lines(), tt.want)
			}
		})
	}
}

func TestHandleHistorySync_SkipsSystemConversations(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	e := historySyncEvent(
		&waHistorySync.Conversation{ID: strPtr("status@broadcast")},
		&waHistorySync.Conversation{ID: strPtr("1@newsletter")},
	)
	c.handleHistorySync(t.Context(), sink, dev, media, e)

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want none of these system JIDs reported as a conversation", sink.Lines())
	}
}

func TestHandleHistorySync_ShowsTheOfficialWhatsAppAccountWhenItHasAMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	conv := &waHistorySync.Conversation{
		ID:       strPtr("0@s.whatsapp.net"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "your security code changed", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 0@s.whatsapp.net WhatsApp") {
		t.Errorf("events = %q, want the \"0\" system account shown as a conversation titled WhatsApp", sink.Lines())
	}
	if !sink.Has("history 0@s.whatsapp.net H1") {
		t.Errorf("events = %q, want its security notice reported", sink.Lines())
	}
}

func TestHandleHistorySync_TitlesTheSelfChat(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.selfJID = types.NewJID("15551234567", types.DefaultUserServer)

	conv := &waHistorySync.Conversation{
		ID:       strPtr("15551234567@s.whatsapp.net"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "note to self", true)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 15551234567@s.whatsapp.net Message yourself") {
		t.Errorf("events = %q, want the self-chat titled \"Message yourself\"", sink.Lines())
	}
	if !sink.Has("history 15551234567@s.whatsapp.net H1") {
		t.Errorf("events = %q, want the self-chat's own message reported", sink.Lines())
	}
}

func TestHandleHistorySync_CollapsesTheSelfChatsLIDFormIntoThePhoneJID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	dev.selfJID, dev.selfLID = types.NewJID("15551234567", types.DefaultUserServer), types.NewJID("111222", types.HiddenUserServer)

	conv := &waHistorySync.Conversation{
		ID:       strPtr("111222@lid"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "reply from the bot", true)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 15551234567@s.whatsapp.net Message yourself") {
		t.Errorf("events = %q, want the LID-addressed self-chat filed under the phone JID", sink.Lines())
	}
	if !sink.Has("history 15551234567@s.whatsapp.net H1") {
		t.Errorf("events = %q, want its message filed under the same phone JID", sink.Lines())
	}
	if sink.Has("conversation 111222@lid Message yourself") {
		t.Errorf("events = %q, want no separate conversation reported under the LID form", sink.Lines())
	}
}

func TestHandleHistorySync_FallsBackToAPhoneNumberForAnUnnamedDirectChat(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	conv := &waHistorySync.Conversation{ID: strPtr("15551234567@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the phone number as a fallback title", sink.Lines())
	}
}

func TestHandleHistorySync_PersistsAMessagesMediaReference(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	hm := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: strPtr("H1")},
		Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			DirectPath: strPtr("/v/abc"), MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3},
			FileLength: u64(99), Mimetype: strPtr("image/jpeg"),
		}},
		MessageTimestamp: u64(1),
	}}
	conv := &waHistorySync.Conversation{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"), Messages: []*waHistorySync.HistorySyncMsg{hm}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	ref, ok, err := media.get(t.Context(), "15551234567@s.whatsapp.net", "H1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("media reference was not saved")
	}
	want := mediaRef{Kind: mediaKindImage, DirectPath: "/v/abc", MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3}, FileLength: 99, Mimetype: "image/jpeg"}
	if !reflect.DeepEqual(ref, want) {
		t.Errorf("media reference = %+v, want %+v", ref, want)
	}
}

func TestHandleHistorySync_DropsAReactionFromHistory(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	hm := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1")},
		Message:          &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommon.MessageKey{ID: strPtr("H0")}, Text: strPtr("👍")}},
		MessageTimestamp: u64(1),
	}}
	conv := &waHistorySync.Conversation{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"), Messages: []*waHistorySync.HistorySyncMsg{hm}}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if sink.Has("history 15551234567@s.whatsapp.net H1") {
		t.Error("a reaction protocol message was reported as history")
	}
}

func TestHandleHistorySync_DropsAConversationWithAnUnparseableID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	conv := &waHistorySync.Conversation{Name: strPtr("Nameless")}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want none for an unparseable id", sink.Lines())
	}
}
