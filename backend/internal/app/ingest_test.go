package app_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestIncomingDedupeEventsReadFocusAndNotificationPolicy(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	direct := addConversation(t, f, "direct", "remote-direct", "Alex", domain.KindDirect)
	addConversation(t, f, "group", "remote-group", "Climbing Crew", domain.KindGroup)

	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-1", SenderID: "alex", SenderName: "Alex", Text: "First", Created: 0})
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "unread.changed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("incoming event order = %v, want %v", got, want)
	}
	calls := f.notifier.Calls()
	if len(calls) != 1 || calls[0] != (notification{Title: "Alex", Body: "First"}) {
		t.Fatalf("direct notification = %#v", calls)
	}
	f.events.reset()
	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-1", SenderName: "Alex", Text: "duplicate", Created: 2})
	if len(f.events.names()) != 0 || len(f.notifier.Calls()) != 1 {
		t.Fatal("duplicate incoming message emitted an event or notification")
	}

	if _, err := f.cmd.ApplySettings(context.Background(), app.SettingsParams{Notifications: true, NotificationPreview: false, DemoChatter: true}); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-2", SenderName: "Alex", Text: "secret body", Created: 3})
	calls = f.notifier.Calls()
	if len(calls) != 2 || calls[1].Body != "New message" {
		t.Errorf("hidden preview notification = %#v", calls)
	}

	f.events.reset()
	if _, err := f.cmd.SetMuted(context.Background(), app.SetMutedParams{ConversationID: direct.ID, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"conversation.updated", "unread.changed"}) {
		t.Errorf("mute events = %v", got)
	}
	f.events.reset()
	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-muted", SenderName: "Alex", Text: "muted", Created: 4})
	if len(f.notifier.Calls()) != 2 {
		t.Error("muted conversation generated a notification")
	}
	if total, _ := f.store.UnreadTotal(); total != 0 {
		t.Errorf("muted unread total = %d, want 0", total)
	}

	if _, err := f.cmd.SetMuted(context.Background(), app.SetMutedParams{ConversationID: direct.ID, Muted: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{ConversationID: direct.ID, WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	f.events.reset()
	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-focused", SenderName: "Alex", Text: "visible in focused chat", Created: 5})
	if total, _ := f.store.UnreadTotal(); total != 0 {
		t.Errorf("focused chat unread = %d, want 0", total)
	}
	if len(f.notifier.Calls()) != 2 {
		t.Error("focused active conversation generated a notification")
	}
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "unread.changed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("focused incoming events = %v, want %v", got, want)
	}

	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{ConversationID: direct.ID, WindowActive: false}); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-inactive", SenderName: "Alex", Text: "window inactive", Created: 6})
	if calls = f.notifier.Calls(); len(calls) != 3 || calls[2].Body != "New message" {
		t.Errorf("inactive window notification = %#v", calls)
	}
	f.ingest.Incoming("wa", "remote-group", domain.Message{RemoteID: "group-in", SenderName: "Priya", Text: "group text", Created: 7})
	if calls = f.notifier.Calls(); len(calls) != 4 || calls[3].Title != "Priya · Climbing Crew" || calls[3].Body != "New message" {
		t.Errorf("group notification = %#v", calls)
	}
	f.ingest.Incoming("wa", "not-present", domain.Message{RemoteID: "ignored", Text: "ignored"})
	if len(f.notifier.Calls()) != 4 {
		t.Error("unknown remote conversation generated a notification")
	}
}

func TestHistoryPersistsUnreadWithoutLiveNotificationsOrFocusReads(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{ConversationID: conv.ID, WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	f.ingest.History("wa", "remote-chat", domain.Message{RemoteID: "history-1", SenderName: "Alex", Text: "historical", Status: domain.StatusReceived, Created: 1})
	updated, err := f.store.Conversation(conv.ID)
	if err != nil || updated.Unread != 1 {
		t.Fatalf("history conversation = %#v, %v", updated, err)
	}
	if len(f.notifier.Calls()) != 0 {
		t.Fatal("history generated a live notification")
	}
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "unread.changed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("history events = %v, want %v", got, want)
	}
	f.events.reset()
	f.ingest.History("wa", "remote-chat", domain.Message{RemoteID: "history-1", Text: "duplicate", Status: domain.StatusReceived, Created: 2})
	if len(f.events.names()) != 0 {
		t.Error("duplicate history message emitted events")
	}
}

func TestSinkPersistsAndEmitsConnectorEvents(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	conv := addConversation(t, f, "chat", "remote-chat", "Original", domain.KindDirect)
	f.ingest.AccountStatus("wa", domain.AccountConnected, "Ready")
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"account.updated"}) {
		t.Fatalf("account status events = %v", got)
	}
	f.events.reset()
	f.ingest.Contact(domain.Contact{AccountID: "wa", RemoteID: "contact", Name: "Contact"})
	contacts, err := f.store.Contacts("wa", "")
	if err != nil || len(contacts) != 1 {
		t.Fatalf("Contact sink did not persist contact: %#v, %v", contacts, err)
	}
	f.ingest.Conversation(domain.Conversation{AccountID: "wa", RemoteID: "remote-chat", Title: "Updated", Kind: domain.KindGroup, Members: 4})
	updated, err := f.store.Conversation(conv.ID)
	if err != nil || updated.Title != "Updated" || updated.Kind != domain.KindGroup || updated.Members != 4 {
		t.Fatalf("Conversation sink = %#v, %v", updated, err)
	}
	f.ingest.Typing("wa", "remote-chat", "Priya", true)
	if got, want := f.events.names(), []string{"conversation.updated", "typing"}; !reflect.DeepEqual(got, want) {
		t.Errorf("conversation/typing events = %v, want %v", got, want)
	}
	f.ingest.AccountStatus("missing", domain.AccountError, "ignored")
	f.ingest.Conversation(domain.Conversation{AccountID: "wa", RemoteID: "", Title: "ignored"})
	f.ingest.Incoming("wa", "missing", domain.Message{Text: "ignored"})
	f.ingest.Typing("wa", "missing", "Nobody", true)
	f.ingest.OutgoingStatus("missing", "remote", domain.StatusSent)

	if _, _, err := f.store.AddMessage(domain.Message{ID: "status-msg", ConversationID: conv.ID, Outgoing: true, Text: "status", Created: 1}); err != nil {
		t.Fatal(err)
	}
	f.events.reset()
	f.ingest.OutgoingStatus("status-msg", "remote-status", domain.StatusSent)
	updatedMessage, err := f.store.Message("status-msg")
	if err != nil || updatedMessage.Status != domain.StatusSent || updatedMessage.RemoteID != "remote-status" {
		t.Fatalf("OutgoingStatus() = %#v, %v", updatedMessage, err)
	}
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"message.updated"}) {
		t.Errorf("OutgoingStatus events = %v", got)
	}
}
