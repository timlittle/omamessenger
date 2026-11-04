package telegram

import (
	"slices"
	"strings"
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

	c.newMessage(t.Context(), &sink, msg, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})
	c.newMessage(t.Context(), &sink, msg, tg.Entities{})

	want := []string{
		"conversation user:42:99 Nadia", "incoming user:42:99 7",
		"incoming user:42:99 7",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q: a peer learned from entities is reused once a later update omits them", got, want)
	}
}

// TestNewMessage_UnresolvedPeerGetsAPlaceholderConversation reproduces
// the bug where a brand-new private chat's first message from a
// stranger never reached the helper: Telegram delivers it either as an
// UpdateShortMessage, which carries no entities at all, or as an
// UpdateNewMessage whose sender is missing from the update's own
// entities because this account has never resolved them before. Either
// way, by the time it reaches newMessage, it looks the same: a message
// whose peer is in neither this call's entities nor this connector's
// own cache of peers already learned (tg.Entities{} and no prior
// c.learn, exactly as a never-before-seen sender's short message or
// bare UpdateNewMessage arrives). Before the fix, newMessage returned
// without reporting anything; now it must still create the
// conversation and store the message under a placeholder name.
func TestNewMessage_UnresolvedPeerGetsAPlaceholderConversation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		peer tg.PeerClass
		want []string
	}{
		{
			"unknown user (a short message, or UpdateNewMessage missing its sender)",
			&tg.PeerUser{UserID: 42},
			[]string{"conversation user:42:0 Telegram user", "incoming user:42:0 7"},
		},
		{
			"unknown chat", &tg.PeerChat{ChatID: 7},
			[]string{"conversation chat:7 Telegram group", "incoming chat:7 7"},
		},
		{
			"unknown channel", &tg.PeerChannel{ChannelID: 5},
			[]string{"conversation channel:5:0 Telegram group", "incoming channel:5:0 7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sink connectortest.Sink
			c := New(domain.Account{ID: "tg"}, "")
			msg := &tg.Message{ID: 7, PeerID: tt.peer, Message: "hello"}

			c.newMessage(t.Context(), &sink, msg, tg.Entities{})

			if got := sink.Lines(); !slices.Equal(got, tt.want) {
				t.Errorf("events = %q, want %q: an unresolved peer must still be stored", got, tt.want)
			}
		})
	}
}

// TestNewMessage_EntitiesResolveTheSameUnknownSenderDifferentWays locks
// in that a brand-new sender's first message is stored identically
// whichever of the two live shapes Telegram uses: an UpdateNewMessage
// whose entities already carry the sender (the common case, when
// Telegram judges the client needs it), and one that omits them, which
// falls back to the placeholder covered above.
func TestNewMessage_EntitiesResolveTheSameUnknownSenderDifferentWays(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		entities tg.Entities
		want     []string
	}{
		{
			"sender in the update's own entities",
			tg.Entities{Users: map[int64]*tg.User{42: nadia()}},
			[]string{"conversation user:42:99 Nadia", "incoming user:42:99 7"},
		},
		{
			"sender missing from the update's entities",
			tg.Entities{},
			[]string{"conversation user:42:0 Telegram user", "incoming user:42:0 7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sink connectortest.Sink
			c := New(domain.Account{ID: "tg"}, "")
			msg := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello"}

			c.newMessage(t.Context(), &sink, msg, tt.entities)

			if got := sink.Lines(); !slices.Equal(got, tt.want) {
				t.Errorf("events = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewMessage_FillsSenderNameFromAnEarlierResponseWhenThisOneCarriesNone
// reproduces a sent reply whose quoted sender showed as "Telegram user"
// instead of the contact's name: Telegram delivers every later message
// in a private chat through its short update form, which carries no
// user entity at all, relying on the client already knowing the
// sender from an earlier, fuller response such as the dialog sync every
// run starts with.
func TestNewMessage_FillsSenderNameFromAnEarlierResponseWhenThisOneCarriesNone(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")

	c.newMessage(t.Context(), &sink, &tg.Message{ID: 6, PeerID: &tg.PeerUser{UserID: 42}, Message: "hey"},
		tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	c.newMessage(t.Context(), &sink, &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}, tg.Entities{})

	live := sink.LiveMessages()["user:42:99"]
	if len(live) != 2 || live[1].SenderName != "Nadia" {
		t.Errorf("second incoming message = %+v, want sender Nadia, not the generic fallback", live)
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

func TestReactionsChanged_ReportsChipsForAKnownChatOnly(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	update := &tg.UpdateMessageReactions{Peer: &tg.PeerUser{UserID: 42}, MsgID: 7, Reactions: tg.MessageReactions{
		Results: []tg.ReactionCount{{Reaction: &tg.ReactionEmoji{Emoticon: "👍"}, Count: 1}},
	}}

	c.reactionsChanged(t.Context(), &sink, update)
	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none for an unknown chat", got)
	}

	c.learn("user:42:99")
	c.reactionsChanged(t.Context(), &sink, update)

	want := []string{"reacted user:42:99 7 1"}
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
			&tg.UpdateMessageReactions{Peer: &tg.PeerUser{UserID: 42}, MsgID: 7, Reactions: tg.MessageReactions{
				Results: []tg.ReactionCount{{Reaction: &tg.ReactionEmoji{Emoticon: "👍"}, Count: 1}},
			}},
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
		"reacted user:42:99 7 1",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

// TestHandleUpdates_GetDifferenceRecoveredNewMessageCreatesTheConversation
// drives the exact envelope gotd's own update manager hands to this
// connector's dispatcher once it has forced updates.getDifference to
// recover a message from a sender it had never seen before: a single
// *tg.Updates whose Updates holds the UpdateNewMessage and whose Users
// carries the recovered sender's full entity, mirroring gotd's own
// TestShortMessageRecoveredDifferenceCarriesSender. This is also the
// shape a missed new chat is recovered in after a reconnect, since
// gotd dispatches a channel or pts gap's catch-up the same way; proving
// it here proves that path too.
func TestHandleUpdates_GetDifferenceRecoveredNewMessageCreatesTheConversation(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")

	d := tg.NewUpdateDispatcher()
	c.handleUpdates(d, &sink)
	err := d.Handle(t.Context(), &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{Message: &tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello"}},
		},
		Users: []tg.UserClass{nadia()},
	})
	if err != nil {
		t.Fatal(err)
	}

	want2 := []string{"conversation user:42:99 Nadia", "incoming user:42:99 10"}
	if got := sink.Lines(); !slices.Equal(got, want2) {
		t.Errorf("events = %q, want %q: a getDifference-recovered new chat must be created and stored", got, want2)
	}
}

// TestNewMessage_LogsAndNeverStoresAServiceMessage confirms a message
// that is not actually text (a group's own system notice, or an
// already-deleted placeholder) is skipped with a category line, never
// silently, and that the line names only the reason, never the peer or
// text.
func TestNewMessage_LogsAndNeverStoresAServiceMessage(t *testing.T) {
	buf := captureLog(t)

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.newMessage(t.Context(), &sink, &tg.MessageEmpty{ID: 9}, tg.Entities{})

	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none", got)
	}

	if got := buf.String(); !strings.Contains(got, "telegram: dropped message (reason=not a text message)") {
		t.Errorf("log = %q, want a dropped-message line naming the reason", got)
	}
}

// TestEditMessage_LogsWhenItsConversationIsNeverResolved confirms an
// edit for a conversation this connector never learned is dropped with
// a category line.
func TestEditMessage_LogsWhenItsConversationIsNeverResolved(t *testing.T) {
	buf := captureLog(t)

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	c.editMessage(t.Context(), &sink, &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "fixed"}, tg.Entities{})

	if got := buf.String(); !strings.Contains(got, "telegram: dropped message (reason=edit for an unresolved conversation)") {
		t.Errorf("log = %q, want a dropped-message line naming the reason", got)
	}
}

// TestNewMessage_UpgradesAPlaceholderWhenALaterMessageResolvesTheSender
// confirms a placeholder created for a stranger's first message, with
// no entities to name them, is upgraded through
// connector.RemoteIDUpgrader as soon as a later message from the same
// person does carry their entity, rather than ending up as a second,
// duplicate conversation once the real one is reported.
func TestNewMessage_UpgradesAPlaceholderWhenALaterMessageResolvesTheSender(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := New(domain.Account{ID: "tg"}, "")

	c.newMessage(t.Context(), sink, &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}, tg.Entities{})
	if len(sink.remoteIDs) != 0 {
		t.Fatalf("remoteIDs after the placeholder = %+v, want none yet", sink.remoteIDs)
	}

	c.newMessage(t.Context(), sink, &tg.Message{ID: 8, PeerID: &tg.PeerUser{UserID: 42}, Message: "it's Nadia"},
		tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	want := []remoteIDChangeCall{{"tg", "user:42:0", "user:42:99"}}
	if !slices.Equal(sink.remoteIDs, want) {
		t.Errorf("remoteIDs = %+v, want %+v", sink.remoteIDs, want)
	}
}

// TestEditMessage_UpgradesACachedPlaceholderWhenItsEntitiesResolve
// confirms an edit whose own entities resolve a peer this connector
// had only ever cached as a placeholder also reports the upgrade,
// the same as a brand-new message's own resolution does: an edit is
// just as able to be the first live update that actually carries the
// sender's entity.
func TestEditMessage_UpgradesACachedPlaceholderWhenItsEntitiesResolve(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := New(domain.Account{ID: "tg"}, "")
	c.learn("user:42:0")

	edit := &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 42}, Message: "fixed"}
	c.editMessage(t.Context(), sink, edit, tg.Entities{Users: map[int64]*tg.User{42: nadia()}})

	want := []remoteIDChangeCall{{"tg", "user:42:0", "user:42:99"}}
	if !slices.Equal(sink.remoteIDs, want) {
		t.Errorf("remoteIDs = %+v, want %+v", sink.remoteIDs, want)
	}
}
