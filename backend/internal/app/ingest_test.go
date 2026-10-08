package app_test

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestIncoming_StoresPublishesAndNotifies(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "First"))

	want := []string{app.EventMessageAdded, app.EventConversationUpdated, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	if got := f.notifier.all(); !slices.Equal(got, []string{"Alex: First"}) {
		t.Errorf("notifications = %v", got)
	}

	// A duplicate changes nothing.
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "again"))
	if len(f.published.take()) != 0 || len(f.notifier.all()) != 1 {
		t.Error("duplicate message published or notified")
	}
}

func TestIncoming_KeepsAPinnedChatPinned(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	// A connector reports the conversation again before an incoming
	// message, as Telegram's live update handling does; the report
	// carries neither field set, the way one built fresh from a peer
	// always does.
	f.ingest.Conversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: chat.RemoteID, Kind: domain.KindDirect, Title: "Alex",
	})
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "First"))

	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || !got.Pinned {
		t.Errorf("Conversation after Incoming = %+v, %v; want Pinned true", got, err)
	}
}

func TestIncoming_NotifiesWithTheConversationID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "First"))

	if got := f.notifier.conversations(); !slices.Equal(got, []string{chat.ID}) {
		t.Errorf("notified conversations = %v, want [%s]", got, chat.ID)
	}
}

func TestIncoming_FollowsSettingsMuteAndFocus(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	group := f.conversation(t, "group", "Climbing Crew", domain.KindGroup)

	f.commands.ApplySettings(app.Settings{Notifications: true, NotificationDetail: "nameOnly"})
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("hidden", "secret"))
	f.ingest.Incoming(ctx, "wa", group.RemoteID, domain.Message{RemoteID: "g", SenderName: "Priya", Text: "hi", Created: 1})

	if _, err := f.commands.SetMuted(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("muted", "quiet"))

	if _, err := f.commands.SetMuted(ctx, chat.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("seen", "looking"))

	want := []string{"Alex: New message", "Priya · Climbing Crew: New message"}
	if got := f.notifier.all(); !slices.Equal(got, want) {
		t.Errorf("notifications = %v, want %v", got, want)
	}

	if seen, _ := f.store.Conversation(ctx, chat.ID); seen.Unread != 0 {
		t.Errorf("focused chat unread = %d, want 0", seen.Unread)
	}
}

// TestIncoming_NotificationDetailControlsHowMuchANotificationShows checks
// the three detail levels directly, and that a Settings value carrying
// only the older NotificationPreview boolean (no NotificationDetail at
// all) still resolves to the matching level, so a UI built before the
// three-level setting existed keeps working.
func TestIncoming_NotificationDetailControlsHowMuchANotificationShows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		settings app.Settings
		want     string
	}{
		{"name and message", app.Settings{Notifications: true, NotificationDetail: "nameAndMessage"}, "Alex: secret"},
		{"name only", app.Settings{Notifications: true, NotificationDetail: "nameOnly"}, "Alex: New message"},
		{"nothing", app.Settings{Notifications: true, NotificationDetail: "none"}, "OmaMessenger: New message"},
		{"migrated from preview true", app.Settings{Notifications: true, NotificationPreview: true}, "Alex: secret"},
		{"migrated from preview false", app.Settings{Notifications: true, NotificationPreview: false}, "Alex: New message"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

			f.commands.ApplySettings(c.settings)
			f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "secret"))

			if got := f.notifier.all(); !slices.Equal(got, []string{c.want}) {
				t.Errorf("notifications = %v, want [%s]", got, c.want)
			}
		})
	}
}

// TestIncoming_ReportsReadToTheServiceWhenFocused reproduces messages
// piling up unread in a conversation the user is actively chatting in:
// marking it read locally is not enough, because the service's own
// unread count, synced back later through Unread, would otherwise still
// be non-zero and resurrect the badge. A burst of messages while focused
// must still produce only one MarkRead call to the service.
func TestIncoming_ReportsReadToTheServiceWhenFocused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))
		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-2", "second"))

		if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
			t.Errorf("unread = %d, want 0", got.Unread)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if got := f.dispatcher.read; !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("read receipts = %v, want one report for %s", got, chat.ID)
		}
	})
}

// TestIncoming_NeverReportsReadWhenNotLookingAtIt checks the service is
// never told a conversation was read while the user is not looking at
// it, even after the debounce window a focused read would use has passed.
func TestIncoming_NeverReportsReadWhenNotLookingAtIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))

		time.Sleep(time.Second)
		synctest.Wait()

		if len(f.dispatcher.read) != 0 {
			t.Errorf("read receipts = %v, want none", f.dispatcher.read)
		}
	})
}

func TestIncoming_IgnoresUnknownConversations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.ingest.Incoming(t.Context(), "wa", "nowhere", incoming("x", "lost"))

	if len(f.published.take()) != 0 || len(f.notifier.all()) != 0 {
		t.Error("message for an unknown conversation published or notified")
	}
}

func TestHistory_NeverNotifiesOrReadsOnArrival(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	m := incoming("h-1", "earlier")
	m.Status = domain.StatusReceived
	f.ingest.History(ctx, "wa", chat.RemoteID, m)
	f.ingest.History(ctx, "wa", "nowhere", m)

	// History never counts towards unread, even for a message that looks
	// unread on its own fields: the service's own count, synced
	// separately through Unread, is authoritative.
	if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
		t.Errorf("unread = %d, want 0", got.Unread)
	}

	if len(f.notifier.all()) != 0 {
		t.Error("history notified")
	}

	want := []string{app.EventMessageAdded, app.EventConversationUpdated}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestEdited_UpdatesTheStoredMessageAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "first"))
	f.published.take()

	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{RemoteID: "e1", Text: "fixed"})

	if got := f.published.take(); !slices.Equal(got, []string{app.EventMessageUpdated}) {
		t.Errorf("events = %v, want %v", got, []string{app.EventMessageUpdated})
	}

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Text != "fixed" || !messages[0].Edited {
		t.Fatalf("messages = %+v, %v; want the text updated and edited set", messages, err)
	}
}

func TestEdited_IgnoresUnknownConversationsAndMessages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	f.ingest.Edited(ctx, "wa", "nowhere", domain.Message{RemoteID: "e1", Text: "x"})
	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{RemoteID: "missing", Text: "x"})

	if len(f.published.take()) != 0 {
		t.Error("an unknown edit published an event")
	}
}

func TestReacted_UpdatesTheStoredMessageAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "hi"))
	f.published.take()

	reactions := []domain.Reaction{{Emoji: "👍", Count: 1, Mine: true}}
	f.ingest.Reacted(ctx, "wa", chat.RemoteID, "e1", reactions)

	if got := f.published.take(); !slices.Equal(got, []string{app.EventMessageUpdated}) {
		t.Errorf("events = %v, want %v", got, []string{app.EventMessageUpdated})
	}

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || !slices.Equal(messages[0].Reactions, reactions) || messages[0].Text != "hi" {
		t.Fatalf("messages = %+v, %v; want the reactions set and the text untouched", messages, err)
	}
}

func TestReacted_IgnoresUnknownConversationsAndMessages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	f.ingest.Reacted(ctx, "wa", "nowhere", "e1", []domain.Reaction{{Emoji: "👍", Count: 1}})
	f.ingest.Reacted(ctx, "wa", chat.RemoteID, "missing", []domain.Reaction{{Emoji: "👍", Count: 1}})

	if len(f.published.take()) != 0 {
		t.Error("an unknown reaction change published an event")
	}
}

func TestDeleted_RemovesMessagesAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("d1", "first"))
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("d2", "second"))
	f.published.take()

	f.ingest.Deleted(ctx, "wa", []string{chat.RemoteID}, []string{"d2"})

	want := []string{app.EventMessageRemoved, app.EventConversationUpdated, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Text != "first" {
		t.Fatalf("messages after delete = %+v, %v", messages, err)
	}

	if got, _ := f.store.Conversation(ctx, chat.ID); got.Preview != "first" || got.Unread != 1 {
		t.Errorf("conversation after delete = %+v", got)
	}
}

func TestDeleted_IgnoresAnEmptyOrUnknownBatch(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.ingest.Deleted(t.Context(), "wa", []string{"nowhere"}, []string{"x"})
	f.ingest.Deleted(t.Context(), "wa", nil, nil)

	if len(f.published.take()) != 0 {
		t.Error("deleting nothing published an event")
	}
}

func TestUnread_TakesTheServicesCount(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 2)
	f.published.take()

	f.ingest.Unread(ctx, "wa", chat.RemoteID, 0)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 0)
	f.ingest.Unread(ctx, "wa", "nowhere", 4)

	if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
		t.Errorf("unread = %d, want 0", got.Unread)
	}

	want := []string{app.EventConversationUpdated, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v once", got, want)
	}
}

// TestUnread_ReMarksReadForTheOpenConversation reproduces the badge
// reappearing on a conversation the user is still looking at: Telegram's
// own count can report non-zero for it, for example because our read
// report is still in flight. That must not be shown, and the service
// should be told again rather than left out of step.
func TestUnread_ReMarksReadForTheOpenConversation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Unread(ctx, "wa", chat.RemoteID, 3)

		if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
			t.Errorf("unread = %d, want 0", got.Unread)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if got := f.dispatcher.read; !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("read receipts = %v, want one report for %s", got, chat.ID)
		}
	})
}

func TestAccountStatus_RecordsAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.ingest.AccountStatus(t.Context(), "wa", domain.AccountConnected, "Ready")
	f.ingest.AccountStatus(t.Context(), "missing", domain.AccountError, "ignored")

	if a := f.published.last(); a.(domain.Account).Status != domain.AccountConnected {
		t.Errorf("published %+v", a)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventAccountUpdated}) {
		t.Errorf("events = %v", got)
	}
}

func TestContactAndConversation_AreRecorded(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	f.ingest.Contact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: "Ben"})
	f.ingest.Conversation(ctx, domain.Conversation{AccountID: "wa", RemoteID: "r1", Title: "Ben", Kind: domain.KindGroup, Members: 4})
	f.ingest.Conversation(ctx, domain.Conversation{AccountID: "wa"}) // invalid: ignored

	if c, err := f.store.Contact(ctx, "wa", "c1"); err != nil || c.Name != "Ben" {
		t.Errorf("contact = %+v, %v", c, err)
	}

	conv, err := f.store.ConversationByRemote(ctx, "wa", "r1")
	if err != nil || conv.Members != 4 {
		t.Errorf("conversation = %+v, %v", conv, err)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}
}

func TestOutgoingStatus_PublishesForwardProgressOnly(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _ := f.commands.Send(ctx, "chat", "hi", "", "")
	f.published.take()

	f.ingest.OutgoingStatus(ctx, m.ID, "remote-1", domain.StatusRead)
	f.ingest.OutgoingStatus(ctx, m.ID, "", domain.StatusDelivered) // late receipt
	f.ingest.OutgoingStatus(ctx, "missing", "", domain.StatusSent)

	if got := f.published.take(); !slices.Equal(got, []string{app.EventMessageUpdated}) {
		t.Errorf("events = %v", got)
	}
}

func TestTyping_PublishesForKnownConversations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	f.ingest.Typing(t.Context(), "wa", chat.RemoteID, "Alex", true)
	f.ingest.Typing(t.Context(), "wa", "nowhere", "Alex", true)

	want := app.Typing{ConversationID: chat.ID, Name: "Alex", Active: true}
	if got := f.published.last(); got != want {
		t.Errorf("typing = %+v, want %+v", got, want)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventTyping}) {
		t.Errorf("events = %v", got)
	}
}
