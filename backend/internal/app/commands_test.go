package app_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestAppMethodsAndConversationWorkflow(t *testing.T) {
	chatterCalls := 0
	chatterValue := false
	f := newFixture(t, fixtureOptions{demo: true, setChatter: func(enabled bool) { chatterCalls++; chatterValue = enabled }})
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	if err := f.store.UpsertContact(domain.Contact{AccountID: "wa", RemoteID: "contact", Name: "New Contact"}); err != nil {
		t.Fatal(err)
	}

	hello, err := f.cmd.Hello(context.Background(), struct{}{})
	if err != nil || hello.Protocol != 1 || hello.Version == "" || !hello.Demo || hello.UnreadTotal != 0 {
		t.Fatalf("Hello() = %#v, %v", hello, err)
	}
	accounts, err := f.cmd.AccountsList(context.Background(), struct{}{})
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa" {
		t.Fatalf("AccountsList() = %#v, %v", accounts, err)
	}
	conversations, err := f.cmd.ConversationsList(context.Background(), app.ConversationsListParams{})
	if err != nil || len(conversations) != 1 || conversations[0].ID != conv.ID {
		t.Fatalf("ConversationsList() = %#v, %v", conversations, err)
	}

	contactList, err := f.cmd.ContactsList(context.Background(), app.ContactsListParams{AccountID: "wa", Query: "contact"})
	if err != nil || len(contactList) != 1 || contactList[0].Name != "New Contact" {
		t.Fatalf("ContactsList() = %#v, %v", contactList, err)
	}
	if _, err := f.cmd.ContactsList(context.Background(), app.ContactsListParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("ContactsList(missing account) error = %v", err)
	}
	if _, err := f.cmd.ContactsList(context.Background(), app.ContactsListParams{AccountID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ContactsList(unknown account) error = %v", err)
	}

	opened, err := f.cmd.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "contact"})
	if err != nil || opened.Title != "New Contact" || opened.Kind != domain.KindDirect {
		t.Fatalf("OpenConversation() = %#v, %v", opened, err)
	}
	if _, err := f.cmd.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "contact"}); err != nil {
		t.Fatalf("opening existing conversation: %v", err)
	}
	if _, err := f.cmd.OpenConversation(context.Background(), app.OpenConversationParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("OpenConversation(missing params) error = %v", err)
	}
	if _, err := f.cmd.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OpenConversation(unknown contact) error = %v", err)
	}

	messages, err := f.cmd.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "chat"})
	if err != nil || len(messages.Messages) != 0 || messages.HasMore {
		t.Fatalf("MessagesList(empty) = %#v, %v", messages, err)
	}
	if _, err := f.cmd.MessagesList(context.Background(), app.MessagesListParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("MessagesList(missing conversation) error = %v", err)
	}
	if _, err := f.cmd.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "chat", Limit: 201}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("MessagesList(invalid limit) error = %v", err)
	}
	if _, err := f.cmd.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MessagesList(unknown conversation) error = %v", err)
	}

	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{ConversationID: "chat", WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{ConversationID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetFocus(unknown conversation) error = %v", err)
	}
	if _, err := f.cmd.SetFocus(context.Background(), app.FocusParams{}); err != nil {
		t.Errorf("SetFocus(clear) error = %v", err)
	}

	if _, err := f.cmd.ApplySettings(context.Background(), app.SettingsParams{Notifications: false, NotificationPreview: false, DemoChatter: false}); err != nil {
		t.Fatal(err)
	}
	if chatterCalls != 1 || chatterValue {
		t.Errorf("demo chatter callback calls=%d enabled=%t", chatterCalls, chatterValue)
	}
	if _, err := f.cmd.Inject(context.Background(), app.InjectParams{ConversationID: "remote-chat"}); !errors.Is(err, app.ErrUnknownMethod) {
		t.Errorf("Inject without demo injector error = %v", err)
	}
	withInjector := f.commands(func(c *app.Config) { c.DemoInject = testInjector{} })
	injected, err := withInjector.Inject(context.Background(), app.InjectParams{ConversationID: "chat"})
	if err != nil || injected.Text != "injected" {
		t.Errorf("Inject() = %#v, %v", injected, err)
	}
	notDemo := f.commands(func(c *app.Config) { c.Demo = false })
	if _, err := notDemo.ApplySettings(context.Background(), app.SettingsParams{DemoChatter: true}); err != nil {
		t.Fatal(err)
	}
	if chatterCalls != 1 {
		t.Errorf("demo chatter callback ran outside demo mode: %d calls", chatterCalls)
	}
}

func TestSendFailureStatusRetryAndValidation(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	f.connector.send = func(_ context.Context, _ domain.Conversation, m domain.Message) error {
		if m.Status == domain.StatusPending {
			return errors.New("offline")
		}
		return nil
	}
	message, err := f.cmd.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: "  send this  "})
	// The connector's immediate send failure is returned directly; verify the
	// stored failed state alongside the error.
	if err == nil || err.Error() != "offline" {
		t.Fatalf("SendMessage() error = %v, want connector failure", err)
	}
	if message.Text != "send this" || !message.Outgoing || message.Status != domain.StatusFailed {
		t.Fatalf("failed SendMessage() = %#v", message)
	}
	stored, err := f.store.Message(message.ID)
	if err != nil || stored.Status != domain.StatusFailed {
		t.Fatalf("stored send = %#v, %v", stored, err)
	}
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "message.updated"}; !reflect.DeepEqual(got, want) {
		t.Errorf("send event order = %v, want %v", got, want)
	}
	f.events.reset()
	f.connector.send = func(_ context.Context, _ domain.Conversation, m domain.Message) error {
		f.ingest.OutgoingStatus(m.ID, "remote-message", domain.StatusSent)
		f.ingest.OutgoingStatus(m.ID, "remote-message", domain.StatusDelivered)
		return nil
	}
	retried, err := f.cmd.Retry(context.Background(), app.RetryParams{MessageID: message.ID})
	if err != nil || retried.Status != domain.StatusPending {
		t.Fatalf("Retry() = %#v, %v", retried, err)
	}
	stored, err = f.store.Message(message.ID)
	if err != nil || stored.Status != domain.StatusDelivered || stored.RemoteID != "remote-message" {
		t.Fatalf("stored retry = %#v, %v", stored, err)
	}
	if got, want := f.events.names(), []string{"message.updated", "message.updated", "message.updated"}; !reflect.DeepEqual(got, want) {
		t.Errorf("retry/status event order = %v, want %v", got, want)
	}

	if _, err := f.cmd.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: "  "}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(empty) error = %v", err)
	}
	if _, err := f.cmd.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: string(make([]byte, 0))}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(empty bytes) error = %v", err)
	}
	if _, err := f.cmd.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "missing", Text: "text"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SendMessage(unknown conversation) error = %v", err)
	}
	if _, err := f.cmd.SendMessage(context.Background(), app.SendMessageParams{Text: "text"}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(missing conversation) error = %v", err)
	}
	if _, err := f.cmd.Retry(context.Background(), app.RetryParams{MessageID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Retry(unknown) error = %v", err)
	}
	if _, err := f.cmd.Retry(context.Background(), app.RetryParams{MessageID: message.ID}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("Retry(non-failed message) error = %v", err)
	}
}

func TestReadMutedAndNoDispatcherPaths(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	for i := 0; i < 2; i++ {
		if _, _, err := f.store.AddMessage(domain.Message{ID: fmt.Sprintf("in-%d", i), ConversationID: conv.ID, Text: "incoming", Created: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.cmd.SetMuted(context.Background(), app.SetMutedParams{ConversationID: conv.ID, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cmd.MarkRead(context.Background(), app.ConversationParams{ConversationID: conv.ID}); err != nil {
		t.Fatal(err)
	}
	updated, err := f.store.Conversation(conv.ID)
	if err != nil || updated.Unread != 0 {
		t.Fatalf("MarkRead() conversation = %#v, %v", updated, err)
	}
	if _, err := f.cmd.MarkRead(context.Background(), app.ConversationParams{ConversationID: "missing"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkRead(unknown) error = %v", err)
	}
	if _, err := f.cmd.SetMuted(context.Background(), app.SetMutedParams{ConversationID: "missing", Muted: true}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetMuted(unknown) error = %v", err)
	}

	noDispatcher, _ := app.New(app.Config{Repo: f.store, Clock: f.clock})
	message, err := noDispatcher.SendMessage(context.Background(), app.SendMessageParams{ConversationID: conv.ID, Text: "queued"})
	if !errors.Is(err, app.ErrNoDispatcher) || message.Status != domain.StatusFailed {
		t.Errorf("SendMessage without dispatcher = %#v, %v", message, err)
	}
	failed := addFailedMessage(t, f, conv.ID, "retry-without-dispatcher")
	retried, err := noDispatcher.Retry(context.Background(), app.RetryParams{MessageID: failed.ID})
	if !errors.Is(err, app.ErrNoDispatcher) || retried.Status != domain.StatusFailed {
		t.Errorf("Retry without dispatcher = %#v, %v", retried, err)
	}
}
