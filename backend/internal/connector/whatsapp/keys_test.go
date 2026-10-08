package whatsapp

// keys_test.go checks that message_keys round-trips a message's
// sender, from-me flag and timestamp, that an older database still
// missing the timestamp column is migrated to have one, that
// unreadMessageKeys and latestMessageKey read back what MarkRead needs
// from it, and that senderKeyID and targetKey turn a saved key into
// the key WhatsApp needs for a direct chat, a group and a message this
// account sent itself.

import (
	"database/sql"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMessageKey_RoundTripsWhatItSaved(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "M1", messageKey{senderID: "sender-1@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	key, found, err := media.messageKeyFor(t.Context(), "chat-1", "M1")
	if err != nil || !found {
		t.Fatalf("messageKeyFor = found=%v err=%v, want it found", found, err)
	}
	if key.senderID != "sender-1@s.whatsapp.net" || key.fromMe {
		t.Errorf("key = %+v, want the saved sender and fromMe", key)
	}
}

func TestMessageKey_UnknownMessageIsNotFound(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)

	_, found, err := media.messageKeyFor(t.Context(), "chat-1", "missing")
	if err != nil || found {
		t.Errorf("messageKeyFor = found=%v err=%v, want not found", found, err)
	}
}

func TestMessageKey_PutReplacesAnEarlierSave(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "M1", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageKey(t.Context(), "chat-1", "M1", messageKey{senderID: "", fromMe: true, timestamp: 2000}); err != nil {
		t.Fatal(err)
	}

	key, found, err := media.messageKeyFor(t.Context(), "chat-1", "M1")
	if err != nil || !found || !key.fromMe || key.senderID != "" {
		t.Errorf("key = %+v found=%v err=%v, want the later save to win", key, found, err)
	}
}

func TestSaveMessageKey_DoesNothingWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	// A nil media store means a test connector, or a run that could not
	// open one, was passed in; saveMessageKey must not panic, it must
	// just skip the save.
	saveMessageKey(t.Context(), nil, "chat-1", "M1", messageKey{senderID: "sender-1@s.whatsapp.net", timestamp: 1000})
}

func TestEnsureMessageKeysTable_AddsTimestampToAnOlderSchema(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const oldSchema = `CREATE TABLE message_keys (
		conversation_id TEXT NOT NULL,
		message_id      TEXT NOT NULL,
		sender_id       TEXT NOT NULL,
		from_me         INTEGER NOT NULL,
		PRIMARY KEY (conversation_id, message_id)
	)`
	if _, err := db.ExecContext(t.Context(), oldSchema); err != nil {
		t.Fatal(err)
	}

	if err := ensureMessageKeysTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	m := &mediaStore{db: db}
	if err := m.putMessageKey(t.Context(), "chat-1", "M1", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1234}); err != nil {
		t.Fatal(err)
	}

	key, found, err := m.messageKeyFor(t.Context(), "chat-1", "M1")
	if err != nil || !found || key.senderID != "a@s.whatsapp.net" {
		t.Errorf("key = %+v found=%v err=%v, want the row saved after the migration added the missing column", key, found, err)
	}
}

func TestUnreadMessageKeys_GroupsTheNewestIncomingIDsBySender(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "old", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageKey(t.Context(), "chat-1", "new", messageKey{senderID: "b@s.whatsapp.net", fromMe: false, timestamp: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageKey(t.Context(), "chat-1", "sent", messageKey{senderID: "", fromMe: true, timestamp: 3000}); err != nil {
		t.Fatal(err)
	}

	bySender, err := media.unreadMessageKeys(t.Context(), "chat-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(bySender) != 2 || len(bySender["a@s.whatsapp.net"]) != 1 || len(bySender["b@s.whatsapp.net"]) != 1 {
		t.Errorf("unread keys = %+v, want one incoming id for each of the two senders, never the sent one", bySender)
	}
}

func TestUnreadMessageKeys_LimitsToTheNewestOnes(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "old", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageKey(t.Context(), "chat-1", "new", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 2000}); err != nil {
		t.Fatal(err)
	}

	bySender, err := media.unreadMessageKeys(t.Context(), "chat-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ids := bySender["a@s.whatsapp.net"]; len(ids) != 1 || ids[0] != "new" {
		t.Errorf("unread keys = %+v, want only the newest id", bySender)
	}
}

func TestUnreadMessageKeys_ZeroLimitReportsNothingWithoutQuerying(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "m1", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	bySender, err := media.unreadMessageKeys(t.Context(), "chat-1", 0)
	if err != nil || len(bySender) != 0 {
		t.Errorf("unread keys = %+v err=%v, want none for a zero limit", bySender, err)
	}
}

func TestLatestMessageKey_ReturnsTheNewestMessageOfAnyKind(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "chat-1", "m1", messageKey{senderID: "a@s.whatsapp.net", fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageKey(t.Context(), "chat-1", "m2", messageKey{senderID: "", fromMe: true, timestamp: 2000}); err != nil {
		t.Fatal(err)
	}

	id, key, found, err := media.latestMessageKey(t.Context(), "chat-1")
	if err != nil || !found {
		t.Fatalf("latestMessageKey = found=%v err=%v, want it found", found, err)
	}
	if id != "m2" || !key.fromMe || key.timestamp != 2000 {
		t.Errorf("latest = id=%q key=%+v, want the newer outgoing message", id, key)
	}
}

func TestLatestMessageKey_NotFoundForAnUnknownConversation(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)

	_, _, found, err := media.latestMessageKey(t.Context(), "chat-1")
	if err != nil || found {
		t.Errorf("latestMessageKey = found=%v err=%v, want not found", found, err)
	}
}

func TestSenderKeyID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info types.MessageInfo
		want string
	}{
		{
			name: "a message from us needs no sender",
			info: types.MessageInfo{MessageSource: types.MessageSource{
				IsFromMe: true, Sender: types.NewJID("15550000000", types.DefaultUserServer),
			}},
			want: "",
		},
		{
			name: "a message from someone else uses their JID",
			info: types.MessageInfo{MessageSource: types.MessageSource{
				Sender: types.NewJID("15551234567", types.DefaultUserServer),
			}},
			want: "15551234567@s.whatsapp.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := senderKeyID(tt.info); got != tt.want {
				t.Errorf("senderKeyID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTargetKey(t *testing.T) {
	t.Parallel()

	direct := types.NewJID("15551234567", types.DefaultUserServer)
	group := types.NewJID("12345-1600000000", types.GroupServer)

	tests := []struct {
		name            string
		chat            types.JID
		key             messageKey
		wantParticipant string
		wantFromMe      bool
	}{
		{
			name:            "a direct chat never sets a participant",
			chat:            direct,
			key:             messageKey{senderID: remoteID(direct), fromMe: false},
			wantParticipant: "",
			wantFromMe:      false,
		},
		{
			name:            "a group message from someone else sets their participant",
			chat:            group,
			key:             messageKey{senderID: "15551234567@s.whatsapp.net", fromMe: false},
			wantParticipant: "15551234567@s.whatsapp.net",
			wantFromMe:      false,
		},
		{
			name:            "a group message we sent sets no participant",
			chat:            group,
			key:             messageKey{fromMe: true},
			wantParticipant: "",
			wantFromMe:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := targetKey(tt.chat, tt.key, "M1")
			if got.GetParticipant() != tt.wantParticipant {
				t.Errorf("participant = %q, want %q", got.GetParticipant(), tt.wantParticipant)
			}
			if got.GetFromMe() != tt.wantFromMe {
				t.Errorf("fromMe = %t, want %t", got.GetFromMe(), tt.wantFromMe)
			}
			if got.GetID() != "M1" || got.GetRemoteJID() != tt.chat.String() {
				t.Errorf("key = %+v, want id M1 and chat %s", got, tt.chat)
			}
		})
	}
}

func TestHandleMessage_SavesTheIncomingMessagesKey(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{Conversation: strPtr("hi")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	key, found, err := media.messageKeyFor(t.Context(), remoteID(liveInfo().Chat), "M1")
	if err != nil || !found {
		t.Fatalf("messageKeyFor = found=%v err=%v, want the live message's key saved", found, err)
	}
	if key.fromMe || key.senderID != remoteID(liveInfo().Sender) {
		t.Errorf("key = %+v, want Nadia's sender id and fromMe false", key)
	}
}

func TestHandleMessage_SavesAFromMeMessagesKeyWithNoSender(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	info := liveInfo()
	info.IsFromMe = true
	e := &events.Message{Info: info, Message: &waE2E.Message{Conversation: strPtr("sent from my phone")}}
	c.handleMessage(t.Context(), &sink, dev, media, e)

	key, found, err := media.messageKeyFor(t.Context(), remoteID(info.Chat), "M1")
	if err != nil || !found || !key.fromMe || key.senderID != "" {
		t.Errorf("key = %+v found=%v err=%v, want fromMe true with no sender", key, found, err)
	}
}

func TestHandleHistorySync_SavesEachMessagesKey(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{
			ID:       strPtr("15551234567@s.whatsapp.net"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	key, found, err := media.messageKeyFor(t.Context(), "15551234567@s.whatsapp.net", "H1")
	if err != nil || !found || key.fromMe {
		t.Errorf("key = %+v found=%v err=%v, want a synced message's key saved", key, found, err)
	}
}
