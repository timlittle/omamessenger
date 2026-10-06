package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSync_ReportsContactsDialogsAndHistory(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContacts{Users: []tg.UserClass{nadia}})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}, UnreadCount: 1}, &tg.DialogFolder{Peer: &tg.PeerUser{UserID: 1}}},
		Users:   []tg.UserClass{nadia},
	})
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 3, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello"}, &tg.MessageEmpty{}},
		Users:    []tg.UserClass{nadia},
	})

	var sink recordingSink
	c := New(domain.Account{ID: "tg"}, "")
	if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
		t.Fatal(err)
	}

	want := []string{"contact user:42:99 Nadia", "conversation user:42:99 Nadia", "history user:42:99 3 hello", "unread user:42:99 1"}
	if got := sink.lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	if _, ok := c.lookup("user:42"); !ok {
		t.Error("synced conversation not remembered for live updates")
	}
}

func TestSync_SkipsUnchangedReplies(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogsNotModified{})

	var sink recordingSink
	c := New(domain.Account{ID: "tg"}, "")
	if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
		t.Fatal(err)
	}

	if got := sink.lines(); len(got) != 0 {
		t.Errorf("events = %q, want none", got)
	}
}

func TestSync_ReportsFailures(t *testing.T) {
	t.Parallel()

	dialogs := &tg.MessagesDialogs{Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}}}, Users: []tg.UserClass{nadia}}
	tests := []struct {
		name    string
		replies func(f *fakeTelegram)
	}{
		{"contacts", func(*fakeTelegram) {}},
		{"dialogs", func(f *fakeTelegram) {
			f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
		}},
		{"history", func(f *fakeTelegram) {
			f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
			f.reply(&tg.MessagesGetDialogsRequest{}, dialogs)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTelegram()
			tt.replies(f)

			c := New(domain.Account{ID: "tg"}, "")
			if err := c.sync(t.Context(), tg.NewClient(f), &recordingSink{}); err == nil {
				t.Error("sync succeeded, want the failure reported")
			}
		})
	}
}

func TestPeerKey_DropsTheAccessHash(t *testing.T) {
	t.Parallel()

	for remote, want := range map[string]string{"user:42:99": "user:42", "chat:7": "chat:7", "odd": "odd"} {
		if got := peerKey(remote); got != want {
			t.Errorf("peerKey(%q) = %q, want %q", remote, got, want)
		}
	}
}
