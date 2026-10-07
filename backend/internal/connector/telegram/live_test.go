package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// nadia is a user the tests receive messages from. It returns a fresh
// value each time: gotd's encoder mutates a message's flags fields while
// encoding, so sharing one pointer across parallel tests would race.
func nadia() *tg.User {
	return &tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia"}
}

func TestNewMessage_ReportsConversationThenMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message tg.MessageClass
		want    []string
	}{
		{
			"incoming", &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"},
			[]string{"conversation user:42:99 Nadia", "incoming user:42:99 7"},
		},
		{
			"ours from another device", &tg.Message{ID: 8, Out: true, PeerID: &tg.PeerUser{UserID: 42}, Message: "yo"},
			[]string{"conversation user:42:99 Nadia", "history user:42:99 8"},
		},
		{"not a message", &tg.MessageEmpty{ID: 9}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sink connectortest.Sink
			c := New(domain.Account{ID: "tg"}, "")
			c.newMessage(t.Context(), &sink, tt.message, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

			if got := sink.Lines(); !slices.Equal(got, tt.want) {
				t.Errorf("events = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewMessage_UsesAKnownPeerWhenTheUpdateOmitsIt(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	msg := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}

	c.newMessage(t.Context(), &sink, msg, tg.Entities{})
	c.learn("user:42:99")
	c.newMessage(t.Context(), &sink, msg, tg.Entities{})

	want := []string{"incoming user:42:99 7"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q: unknown peers are dropped, known ones used", got, want)
	}
}

func TestEditMessage_ReportsAnEditedMessageInAKnownConversation(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	edit := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "fixed"}

	// Not in entities and never learned: dropped.
	c.editMessage(t.Context(), &sink, edit, tg.Entities{})
	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none for an unknown peer", got)
	}

	c.editMessage(t.Context(), &sink, edit, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})
	want := []string{"edited user:42:99 7"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	sink = connectortest.Sink{}
	c.learn("user:42:99")
	c.editMessage(t.Context(), &sink, edit, tg.Entities{})
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q for a known peer", got, want)
	}
}

func TestEditMessage_IgnoresAnUpdateThatIsNotAMessage(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.editMessage(t.Context(), &sink, &tg.MessageEmpty{ID: 9}, tg.Entities{})

	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none", got)
	}
}

func TestDeleteMessages_ReportsIDsScopedToTheGivenConversations(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.deleteMessages(t.Context(), &sink, []string{"user:42:99", "chat:7"}, []int{7, 8})

	want := []string{"deleted user:42:99,chat:7 7,8"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestDeleteMessages_IgnoresAnEmptyBatchOrScope(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.deleteMessages(t.Context(), &sink, []string{"chat:1"}, nil)
	c.deleteMessages(t.Context(), &sink, nil, []int{1})

	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none", got)
	}
}

// TestHandleUpdates_DeleteMessagesScopesToNonChannelConversations
// reproduces a bug: Telegram numbers a channel's messages in their own
// space, so a deletion with no peer (which only private chats and basic
// groups can raise) must never be scoped to a channel, even one this
// connector has learned about, or a coincidentally matching channel
// message would be deleted along with the intended one.
func TestHandleUpdates_DeleteMessagesScopesToNonChannelConversations(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.learn("user:42:99")
	c.learn("chat:7")
	c.learn("channel:5:3")

	d := tg.NewUpdateDispatcher()
	c.handleUpdates(d, &sink)
	err := d.Handle(t.Context(), &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateDeleteMessages{Messages: []int{5}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"deleted chat:7,user:42:99 5"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q: a channel must never share this scope", got, want)
	}
}

func TestReadUpTo_MarksOurMessagesRead(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.learn("user:42:99")
	c.remember("user:42:99", "10", "m1")
	c.remember("user:42:99", "12", "m2")
	c.remember("chat:7", "5", "m3")

	c.readUpTo(t.Context(), &sink, "user:43", 20)
	c.readUpTo(t.Context(), &sink, "user:42", 11)
	c.readUpTo(t.Context(), &sink, "user:42", 11)

	want := []string{"outgoing m1  " + domain.StatusRead}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestTyping_ReportsStartAndStopInKnownChats(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.typing(t.Context(), &sink, &tg.UpdateUserTyping{UserID: 42, Action: &tg.SendMessageTypingAction{}})
	c.learn("user:42:99")
	c.typing(t.Context(), &sink, &tg.UpdateUserTyping{UserID: 42, Action: &tg.SendMessageTypingAction{}})
	c.typing(t.Context(), &sink, &tg.UpdateUserTyping{UserID: 42, Action: &tg.SendMessageCancelAction{}})

	want := []string{"typing user:42:99 true", "typing user:42:99 false"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestShortKey_NamesEachKindOfPeer(t *testing.T) {
	t.Parallel()

	peers := []tg.PeerClass{&tg.PeerUser{UserID: 1}, &tg.PeerChat{ChatID: 2}, &tg.PeerChannel{ChannelID: 3}, nil}
	want := []string{"user:1", "chat:2", "channel:3", ""}
	for i, p := range peers {
		if got := shortKey(p); got != want[i] {
			t.Errorf("shortKey(%T) = %q, want %q", p, got, want[i])
		}
	}
}

func TestHandleUpdates_RoutesEachKindOfUpdate(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.learn("channel:5:3")
	c.remember("user:42:99", "8", "m1")

	d := tg.NewUpdateDispatcher()
	c.handleUpdates(d, &sink)
	err := d.Handle(t.Context(), &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{Message: &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}},
			&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 3, PeerID: &tg.PeerChannel{ChannelID: 5}, Message: "news"}},
			&tg.UpdateReadHistoryOutbox{Peer: &tg.PeerUser{UserID: 42}, MaxID: 8},
			&tg.UpdateUserTyping{UserID: 42, Action: &tg.SendMessageTypingAction{}},
			&tg.UpdateReadHistoryInbox{Peer: &tg.PeerUser{UserID: 42}, StillUnreadCount: 2},
			&tg.UpdateReadChannelInbox{ChannelID: 5, StillUnreadCount: 0},
			&tg.UpdateReadChannelInbox{ChannelID: 6, StillUnreadCount: 0},
			&tg.UpdateEditMessage{Message: &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi edited"}},
			&tg.UpdateEditChannelMessage{Message: &tg.Message{ID: 3, PeerID: &tg.PeerChannel{ChannelID: 5}, Message: "news edited"}},
			&tg.UpdateDeleteMessages{Messages: []int{8, 9}},
			&tg.UpdateDeleteChannelMessages{ChannelID: 5, Messages: []int{3}},
			&tg.UpdateDeleteChannelMessages{ChannelID: 6, Messages: []int{1}},
		},
		Users: []tg.UserClass{nadia()},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"conversation user:42:99 Nadia", "incoming user:42:99 7",
		"incoming channel:5:3 3",
		"outgoing m1  " + domain.StatusRead,
		"typing user:42:99 true",
		"unread user:42:99 2",
		"unread channel:5:3 0",
		"edited user:42:99 7",
		"edited channel:5:3 3",
		"deleted user:42:99 8,9",
		"deleted channel:5:3 3",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}
