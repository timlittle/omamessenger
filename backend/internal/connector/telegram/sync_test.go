package telegram

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSync_ListsEveryChatBeforeLoadingHistory(t *testing.T) {
	t.Parallel()

	// Loading Nadia's history fails, as it does for a chat the user has
	// left; the group after her must still be listed and loaded.
	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContacts{Users: []tg.UserClass{nadia()}})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}, TopMessage: 9, UnreadCount: 1},
			&tg.DialogFolder{Peer: &tg.PeerUser{UserID: 1}},
			&tg.Dialog{Peer: &tg.PeerChat{ChatID: 7}, TopMessage: 4},
		},
		Messages: []tg.MessageClass{
			&tg.Message{ID: 9, PeerID: &tg.PeerUser{UserID: 42}, Message: "latest"},
			&tg.Message{ID: 4, PeerID: &tg.PeerChat{ChatID: 7}, Message: "top"},
		},
		Chats: []tg.ChatClass{&tg.Chat{ID: 7, Title: "Crew", Photo: &tg.ChatPhotoEmpty{}}},
		Users: []tg.UserClass{nadia()},
	})
	f.failNext(&tg.MessagesGetHistoryRequest{}, tgerr.New(400, "CHANNEL_PRIVATE"))
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 3, PeerID: &tg.PeerChat{ChatID: 7}, Message: "older"}, &tg.MessageEmpty{}},
	})

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"contact user:42:99 Nadia",
		"conversation user:42:99 Nadia", "organized user:42:99 false false", "history user:42:99 9", "unread user:42:99 1",
		"conversation chat:7 Crew", "organized chat:7 false false", "history chat:7 4", "unread chat:7 0",
		"history chat:7 3", "unread chat:7 0",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events =\n%q\nwant\n%q", got, want)
	}

	if _, ok := c.lookup("user:42"); !ok {
		t.Error("synced conversation not remembered for live updates")
	}
}

func TestSync_ReportsOrganizedForPinnedAndArchivedDialogs(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}, Pinned: true},
			&tg.Dialog{Peer: &tg.PeerChat{ChatID: 7}, FolderID: archiveFolderID},
		},
		Chats: []tg.ChatClass{&tg.Chat{ID: 7, Title: "Crew", Photo: &tg.ChatPhotoEmpty{}}},
		Users: []tg.UserClass{nadia()},
	})
	f.failNext(&tg.MessagesGetHistoryRequest{}, tgerr.New(400, "CHANNEL_PRIVATE"))
	f.failNext(&tg.MessagesGetHistoryRequest{}, tgerr.New(400, "CHANNEL_PRIVATE"))

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
		t.Fatal(err)
	}

	want := []string{"organized user:42:99 true false", "organized chat:7 false true"}
	var got []string
	for _, line := range sink.Lines() {
		if strings.HasPrefix(line, "organized ") {
			got = append(got, line)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("organized lines = %q, want %q", got, want)
	}
}

func TestNewMessage_NeverReportsOrganized(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.newMessage(t.Context(), &sink, &tg.Message{
		ID: 1, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi",
	}, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	for _, line := range sink.Lines() {
		if strings.HasPrefix(line, "organized ") {
			t.Errorf("a live message reported %q, want it to leave pinned and archived alone", line)
		}
	}
}

func TestSync_WaitsOutRateLimits(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
		f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}}},
			Users:   []tg.UserClass{nadia()},
		})
		f.failNext(&tg.MessagesGetHistoryRequest{}, tgerr.New(420, "FLOOD_WAIT_3"))
		f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
			Messages: []tg.MessageClass{&tg.Message{ID: 3, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello"}},
		})

		var sink connectortest.Sink
		start := time.Now()
		c := New(domain.Account{ID: "tg"}, "")
		if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
			t.Fatal(err)
		}

		if waited := time.Since(start); waited < 3*time.Second {
			t.Errorf("waited %v, want the 3s Telegram asked for", waited)
		}

		if !slices.Contains(sink.Lines(), "history user:42:99 3") {
			t.Errorf("history not loaded after the wait: %q", sink.Lines())
		}
	})
}

func TestSync_SkipsUnchangedReplies(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogsNotModified{})

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	if err := c.sync(t.Context(), tg.NewClient(f), &sink); err != nil {
		t.Fatal(err)
	}

	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none", got)
	}
}

func TestSync_ReportsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		replies func(f *fakeTelegram)
	}{
		{"contacts", func(*fakeTelegram) {}},
		{"dialogs", func(f *fakeTelegram) {
			f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTelegram()
			tt.replies(f)

			c := New(domain.Account{ID: "tg"}, "")
			if err := c.sync(t.Context(), tg.NewClient(f), &connectortest.Sink{}); err == nil {
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

func TestLoadOlder_FetchesFromBeforeTheOldestMessage(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 39, PeerID: &tg.PeerUser{UserID: 42}, Message: "older"},
			&tg.Message{ID: 38, PeerID: &tg.PeerUser{UserID: 42}, Message: "oldest"},
		},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	n, err := c.LoadOlder(t.Context(), chatWithNadia, "40", 30)
	if err != nil || n != 2 {
		t.Fatalf("LoadOlder = %d, %v; want 2", n, err)
	}

	req, ok := f.sent()[0].(*tg.MessagesGetHistoryRequest)
	if !ok || req.OffsetID != 40 || req.Limit != 30 {
		t.Errorf("request = %+v, want 30 messages before 40", f.sent()[0])
	}

	want := []string{"history user:42:99 39", "history user:42:99 38"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestLoadOlder_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":     errOf(New(domain.Account{ID: "tg"}, "").LoadOlder(t.Context(), chatWithNadia, "40", 30)),
		"from a malformed id":   errOf(connected.LoadOlder(t.Context(), chatWithNadia, "x", 30)),
		"for a malformed peer":  errOf(connected.LoadOlder(t.Context(), domain.Conversation{RemoteID: "bad"}, "40", 30)),
		"when Telegram refuses": errOf(connected.LoadOlder(t.Context(), chatWithNadia, "40", 30)),
	} {
		if err == nil {
			t.Errorf("LoadOlder %s succeeded, want an error", name)
		}
	}
}

// errOf keeps the error of a call that also returns a count.
func errOf(_ int, err error) error {
	return err
}
