package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// testEntities are a user, a basic group and a supergroup.
func testEntities() entities {
	return newEntities(
		[]tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia", LastName: "Rahman"}},
		[]tg.ChatClass{
			&tg.Chat{ID: 7, Title: "Climbing Crew", ParticipantsCount: 5},
			&tg.Channel{ID: 5, AccessHash: 3, Title: "Omarchy Users", Megagroup: true, ParticipantsCount: 2400},
		},
	)
}

func TestConversation_FromEachKindOfDialog(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name   string
		dialog *tg.Dialog
		want   domain.Conversation
	}{
		{
			"direct chat", &tg.Dialog{Peer: &tg.PeerUser{UserID: 42}},
			domain.Conversation{AccountID: "tg", RemoteID: "user:42:99", Kind: domain.KindDirect, Title: "Nadia Rahman"},
		},
		{
			"basic group", &tg.Dialog{Peer: &tg.PeerChat{ChatID: 7}},
			domain.Conversation{AccountID: "tg", RemoteID: "chat:7", Kind: domain.KindGroup, Title: "Climbing Crew", Members: 5},
		},
		{
			"muted supergroup", &tg.Dialog{
				Peer:           &tg.PeerChannel{ChannelID: 5},
				NotifySettings: tg.PeerNotifySettings{MuteUntil: int(now.Unix()) + 3600},
			},
			domain.Conversation{AccountID: "tg", RemoteID: "channel:5:3", Kind: domain.KindGroup, Title: "Omarchy Users", Members: 2400, Muted: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := conversation("tg", tt.dialog, testEntities(), now)
			if !ok || got != tt.want {
				t.Errorf("conversation = %+v, %t; want %+v", got, ok, tt.want)
			}
		})
	}
}

func TestConversation_NeverSetsPinnedOrArchived(t *testing.T) {
	t.Parallel()

	// Pinned and archived reach the store only through Sink.Organized
	// (see listDialogs), never through the conversation a dialog
	// otherwise reports, so a later bare report can never clear them.
	now := time.Now()
	dialog := &tg.Dialog{Peer: &tg.PeerUser{UserID: 42}, Pinned: true, FolderID: archiveFolderID}

	got, ok := conversation("tg", dialog, testEntities(), now)
	if !ok || got.Pinned || got.Archived {
		t.Errorf("conversation = %+v, %t; want neither Pinned nor Archived set", got, ok)
	}
}

func TestConversation_SkipsUnknownPeers(t *testing.T) {
	t.Parallel()

	if _, ok := conversation("tg", &tg.Dialog{Peer: &tg.PeerUser{UserID: 1}}, testEntities(), time.Now()); ok {
		t.Error("a dialog with an unknown user became a conversation")
	}
}

func TestMessage_FromIncomingOutgoingAndMedia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *tg.Message
		want domain.Message
	}{
		{
			"incoming in a group",
			&tg.Message{ID: 10, Date: 1_800_000_000, Message: "hi", PeerID: &tg.PeerChat{ChatID: 7}, FromID: &tg.PeerUser{UserID: 42}},
			domain.Message{RemoteID: "10", SenderID: "42", SenderName: "Nadia Rahman", Text: "hi", Status: domain.StatusReceived, Created: 1_800_000_000_000},
		},
		{
			"incoming direct, sender from the peer",
			&tg.Message{ID: 11, Date: 1, Message: "yo", PeerID: &tg.PeerUser{UserID: 42}},
			domain.Message{RemoteID: "11", SenderID: "42", SenderName: "Nadia Rahman", Text: "yo", Status: domain.StatusReceived, Created: 1000},
		},
		{
			"outgoing",
			&tg.Message{ID: 12, Date: 2, Out: true, Message: "back", PeerID: &tg.PeerUser{UserID: 42}},
			domain.Message{RemoteID: "12", SenderID: "self", SenderName: "You", Text: "back", Outgoing: true, Status: domain.StatusSent, Created: 2000},
		},
		{
			"photo with no caption",
			&tg.Message{ID: 13, Date: 3, Media: &tg.MessageMediaPhoto{}, PeerID: &tg.PeerUser{UserID: 42}},
			domain.Message{RemoteID: "13", SenderID: "42", SenderName: "Nadia Rahman", Text: "[Photo]", Status: domain.StatusReceived, Created: 3000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := message(tt.msg, testEntities()); got != tt.want {
				t.Errorf("message = %+v\nwant      %+v", got, tt.want)
			}
		})
	}
}

func TestMessage_ReportsTheMessageItReplies(t *testing.T) {
	t.Parallel()

	header := &tg.MessageReplyHeader{}
	header.SetReplyToMsgID(7)
	withReply := &tg.Message{ID: 20, Date: 1, Message: "sure", PeerID: &tg.PeerUser{UserID: 42}, ReplyTo: header}

	got := message(withReply, testEntities())
	if got.ReplyTo == nil || got.ReplyTo.RemoteID != "7" {
		t.Fatalf("message.ReplyTo = %+v, want it to quote remote id 7", got.ReplyTo)
	}

	plain := message(&tg.Message{ID: 21, Date: 1, Message: "hi", PeerID: &tg.PeerUser{UserID: 42}}, testEntities())
	if plain.ReplyTo != nil {
		t.Errorf("message.ReplyTo = %+v, want nil for a message that answers nothing", plain.ReplyTo)
	}
}

func TestUserName_FallsBackSensibly(t *testing.T) {
	t.Parallel()

	tests := map[string]*tg.User{
		"Ada":            {FirstName: "Ada"},
		"@ada":           {Username: "ada"},
		"Telegram user":  {},
		"Ada Lovelace":   {FirstName: "Ada", LastName: "Lovelace"},
		"Saved Messages": {Self: true, FirstName: "Me"},
	}

	for want, u := range tests {
		if got := userName(u); got != want {
			t.Errorf("userName(%+v) = %q, want %q", u, got, want)
		}
	}
}

func TestOwnName_IsThePersonNotTheirSavedMessages(t *testing.T) {
	t.Parallel()

	if got := ownName(&tg.User{Self: true, FirstName: "Tim"}); got != "Tim" {
		t.Errorf("ownName = %q, want Tim", got)
	}
}

// FuzzMessage checks that message never panics on a message Telegram
// sends, whatever its id, date, text or outgoing flag, and always returns
// some text, since every stored message needs one.
func FuzzMessage(f *testing.F) {
	f.Add(10, "hi", false, 1_800_000_000)
	f.Add(12, "", true, 2)
	f.Add(13, "  ", false, 3)

	f.Fuzz(func(t *testing.T, id int, text string, out bool, date int) {
		msg := &tg.Message{ID: id, Date: date, Message: text, Out: out, PeerID: &tg.PeerUser{UserID: 42}}
		got := message(msg, testEntities())
		if got.Text == "" {
			t.Errorf("message(%+v) produced no text", msg)
		}
	})
}

// FuzzMessageText checks that messageText never panics on a message's raw
// text and passes real text through unchanged.
func FuzzMessageText(f *testing.F) {
	for _, text := range []string{"", " ", "hello", "  multi\nline  ", "emoji 👍"} {
		f.Add(text)
	}

	f.Fuzz(func(t *testing.T, text string) {
		got := messageText(&tg.Message{Message: text})
		if strings.TrimSpace(text) != "" && got != text {
			t.Errorf("messageText(%q) = %q, want it unchanged", text, got)
		}
	})
}

func TestMessageText_LabelsMediaWithoutACaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		media tg.MessageMediaClass
		want  string
	}{
		{&tg.MessageMediaDocument{}, "[File]"},
		{&tg.MessageMediaGeo{}, "[Location]"},
		{&tg.MessageMediaVenue{}, "[Location]"},
		{&tg.MessageMediaContact{}, "[Contact]"},
		{&tg.MessageMediaPoll{}, "[Poll]"},
		{nil, "[Message]"},
	}

	for _, tt := range tests {
		if got := messageText(&tg.Message{Message: " ", Media: tt.media}); got != tt.want {
			t.Errorf("messageText(%T) = %q, want %q", tt.media, got, tt.want)
		}
	}
}
