package app_test

import (
	"slices"
	"testing"

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

func TestIncoming_FollowsSettingsMuteAndFocus(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	group := f.conversation(t, "group", "Climbing Crew", domain.KindGroup)

	f.commands.ApplySettings(app.Settings{Notifications: true, NotificationPreview: false})
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

	if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 1 {
		t.Errorf("unread = %d, want 1", got.Unread)
	}

	if len(f.notifier.all()) != 0 {
		t.Error("history notified")
	}

	want := []string{app.EventMessageAdded, app.EventConversationUpdated, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
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
	m, _ := f.commands.Send(ctx, "chat", "hi")
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
