package app_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

type event struct {
	name string
	data any
}

type eventLog struct {
	mu     sync.Mutex
	events []event
}

func (l *eventLog) add(name string, data any) {
	l.mu.Lock()
	l.events = append(l.events, event{name: name, data: data})
	l.mu.Unlock()
}

func (l *eventLog) names() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, len(l.events))
	for i, e := range l.events {
		names[i] = e.name
	}
	return names
}

func (l *eventLog) reset() {
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
}

type testConnector struct {
	account  domain.Account
	send     func(context.Context, domain.Conversation, domain.Message) error
	markRead func(context.Context, domain.Conversation) error
}

func (c *testConnector) Account() domain.Account { return c.account }
func (*testConnector) Run(ctx context.Context, _ connector.Sink) error {
	<-ctx.Done()
	return nil
}
func (c *testConnector) Send(ctx context.Context, conv domain.Conversation, message domain.Message) error {
	if c.send != nil {
		return c.send(ctx, conv, message)
	}
	return nil
}
func (c *testConnector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	if c.markRead != nil {
		return c.markRead(ctx, conv)
	}
	return nil
}

type fixture struct {
	app       *app.App
	store     *store.Store
	connector *testConnector
	clock     *clocktest.Clock
	notifier  *notify.Recorder
	events    *eventLog
}

func newFixture(t *testing.T, demo bool) *fixture {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data", "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	clock := clocktest.New(time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC))
	notifier := &notify.Recorder{}
	appService := app.New(db, nil, notifier, clock)
	appService.Demo = demo
	events := &eventLog{}
	appService.Emit = events.add
	conn := &testConnector{account: domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}}
	manager := &connector.Manager{Store: db, Sink: appService, Clock: clock, Connectors: []connector.Connector{conn}}
	appService.Manager = manager
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		manager.Wait()
	})
	return &fixture{app: appService, store: db, connector: conn, clock: clock, notifier: notifier, events: events}
}

func addConversation(t *testing.T, f *fixture, id, remote, title, kind string) domain.Conversation {
	t.Helper()
	conv, _, err := f.store.EnsureConversation(domain.Conversation{
		ID: id, AccountID: "wa", RemoteID: remote, Title: title, Kind: kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	return conv
}

func addFailedMessage(t *testing.T, f *fixture, convID, id string) domain.Message {
	t.Helper()
	m, _, err := f.store.AddMessage(domain.Message{
		ID: id, ConversationID: convID, Text: "outgoing", Outgoing: true, Status: domain.StatusPending, Created: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, changed, err := f.store.UpdateMessageStatus(m.ID, domain.StatusFailed)
	if err != nil || !changed {
		t.Fatalf("make failed message: %#v changed=%t err=%v", m, changed, err)
	}
	return m
}

func TestAppMethodsAndConversationWorkflow(t *testing.T) {
	f := newFixture(t, true)
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	if err := f.store.UpsertContact(domain.Contact{AccountID: "wa", RemoteID: "contact", Name: "New Contact"}); err != nil {
		t.Fatal(err)
	}

	hello, err := f.app.Hello(context.Background(), struct{}{})
	if err != nil || hello.Protocol != 1 || hello.Version == "" || !hello.Demo || hello.UnreadTotal != 0 {
		t.Fatalf("Hello() = %#v, %v", hello, err)
	}
	accounts, err := f.app.AccountsList(context.Background(), struct{}{})
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa" {
		t.Fatalf("AccountsList() = %#v, %v", accounts, err)
	}
	conversations, err := f.app.ConversationsList(context.Background(), app.ConversationsListParams{})
	if err != nil || len(conversations) != 1 || conversations[0].ID != conv.ID {
		t.Fatalf("ConversationsList() = %#v, %v", conversations, err)
	}

	contactList, err := f.app.ContactsList(context.Background(), app.ContactsListParams{AccountID: "wa", Query: "contact"})
	if err != nil || len(contactList) != 1 || contactList[0].Name != "New Contact" {
		t.Fatalf("ContactsList() = %#v, %v", contactList, err)
	}
	if _, err := f.app.ContactsList(context.Background(), app.ContactsListParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("ContactsList(missing account) error = %v", err)
	}
	if _, err := f.app.ContactsList(context.Background(), app.ContactsListParams{AccountID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ContactsList(unknown account) error = %v", err)
	}

	opened, err := f.app.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "contact"})
	if err != nil || opened.Title != "New Contact" || opened.Kind != domain.KindDirect {
		t.Fatalf("OpenConversation() = %#v, %v", opened, err)
	}
	if _, err := f.app.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "contact"}); err != nil {
		t.Fatalf("opening existing conversation: %v", err)
	}
	if _, err := f.app.OpenConversation(context.Background(), app.OpenConversationParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("OpenConversation(missing params) error = %v", err)
	}
	if _, err := f.app.OpenConversation(context.Background(), app.OpenConversationParams{AccountID: "wa", ContactID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("OpenConversation(unknown contact) error = %v", err)
	}

	messages, err := f.app.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "chat"})
	if err != nil || len(messages.Messages) != 0 || messages.HasMore {
		t.Fatalf("MessagesList(empty) = %#v, %v", messages, err)
	}
	if _, err := f.app.MessagesList(context.Background(), app.MessagesListParams{}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("MessagesList(missing conversation) error = %v", err)
	}
	if _, err := f.app.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "chat", Limit: 201}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("MessagesList(invalid limit) error = %v", err)
	}
	if _, err := f.app.MessagesList(context.Background(), app.MessagesListParams{ConversationID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("MessagesList(unknown conversation) error = %v", err)
	}

	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{ConversationID: "chat", WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{ConversationID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetFocus(unknown conversation) error = %v", err)
	}
	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{}); err != nil {
		t.Errorf("SetFocus(clear) error = %v", err)
	}

	chatterCalls := 0
	chatterValue := false
	f.app.SetChatter = func(enabled bool) { chatterCalls++; chatterValue = enabled }
	if _, err := f.app.ApplySettings(context.Background(), app.SettingsParams{Notifications: false, NotificationPreview: false, DemoChatter: false}); err != nil {
		t.Fatal(err)
	}
	if chatterCalls != 1 || chatterValue {
		t.Errorf("demo chatter callback calls=%d enabled=%t", chatterCalls, chatterValue)
	}
	if _, err := f.app.Inject(context.Background(), app.InjectParams{ConversationID: "remote-chat"}); !errors.Is(err, app.ErrUnknownMethod) {
		t.Errorf("Inject without demo injector error = %v", err)
	}
	f.app.DemoInject = testInjector{}
	injected, err := f.app.Inject(context.Background(), app.InjectParams{ConversationID: "remote-chat"})
	if err != nil || injected.Text != "injected" {
		t.Errorf("Inject() = %#v, %v", injected, err)
	}
	f.app.Demo = false
	if _, err := f.app.ApplySettings(context.Background(), app.SettingsParams{DemoChatter: true}); err != nil {
		t.Fatal(err)
	}
	if chatterCalls != 1 {
		t.Errorf("demo chatter callback ran outside demo mode: %d calls", chatterCalls)
	}
}

type testInjector struct{}

func (testInjector) Inject(remoteID string) (domain.Message, error) {
	return domain.Message{ConversationID: remoteID, Text: "injected"}, nil
}

func TestSendFailureStatusRetryAndValidation(t *testing.T) {
	f := newFixture(t, false)
	addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	f.connector.send = func(_ context.Context, _ domain.Conversation, m domain.Message) error {
		if m.Status == domain.StatusPending {
			return errors.New("offline")
		}
		return nil
	}
	message, err := f.app.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: "  send this  "})
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
		f.app.OutgoingStatus(m.ID, "remote-message", domain.StatusSent)
		f.app.OutgoingStatus(m.ID, "remote-message", domain.StatusDelivered)
		return nil
	}
	retried, err := f.app.Retry(context.Background(), app.RetryParams{MessageID: message.ID})
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

	if _, err := f.app.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: "  "}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(empty) error = %v", err)
	}
	if _, err := f.app.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "chat", Text: string(make([]byte, 0))}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(empty bytes) error = %v", err)
	}
	if _, err := f.app.SendMessage(context.Background(), app.SendMessageParams{ConversationID: "missing", Text: "text"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SendMessage(unknown conversation) error = %v", err)
	}
	if _, err := f.app.SendMessage(context.Background(), app.SendMessageParams{Text: "text"}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("SendMessage(missing conversation) error = %v", err)
	}
	if _, err := f.app.Retry(context.Background(), app.RetryParams{MessageID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Retry(unknown) error = %v", err)
	}
	if _, err := f.app.Retry(context.Background(), app.RetryParams{MessageID: message.ID}); !errors.Is(err, app.ErrBadRequest) {
		t.Errorf("Retry(non-failed message) error = %v", err)
	}
}

func TestIncomingDedupeEventsReadFocusAndNotificationPolicy(t *testing.T) {
	f := newFixture(t, false)
	direct := addConversation(t, f, "direct", "remote-direct", "Alex", domain.KindDirect)
	addConversation(t, f, "group", "remote-group", "Climbing Crew", domain.KindGroup)

	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-1", SenderID: "alex", SenderName: "Alex", Text: "First", Created: 0})
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "unread.changed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("incoming event order = %v, want %v", got, want)
	}
	calls := f.notifier.Calls()
	if len(calls) != 1 || calls[0] != (notify.Notification{Title: "Alex", Body: "First"}) {
		t.Fatalf("direct notification = %#v", calls)
	}
	f.events.reset()
	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-1", SenderName: "Alex", Text: "duplicate", Created: 2})
	if len(f.events.names()) != 0 || len(f.notifier.Calls()) != 1 {
		t.Fatal("duplicate incoming message emitted an event or notification")
	}

	if _, err := f.app.ApplySettings(context.Background(), app.SettingsParams{Notifications: true, NotificationPreview: false, DemoChatter: true}); err != nil {
		t.Fatal(err)
	}
	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-2", SenderName: "Alex", Text: "secret body", Created: 3})
	calls = f.notifier.Calls()
	if len(calls) != 2 || calls[1].Body != "New message" {
		t.Errorf("hidden preview notification = %#v", calls)
	}

	f.events.reset()
	if _, err := f.app.SetMuted(context.Background(), app.SetMutedParams{ConversationID: direct.ID, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"conversation.updated", "unread.changed"}) {
		t.Errorf("mute events = %v", got)
	}
	f.events.reset()
	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-muted", SenderName: "Alex", Text: "muted", Created: 4})
	if len(f.notifier.Calls()) != 2 {
		t.Error("muted conversation generated a notification")
	}
	if total, _ := f.store.UnreadTotal(); total != 0 {
		t.Errorf("muted unread total = %d, want 0", total)
	}

	if _, err := f.app.SetMuted(context.Background(), app.SetMutedParams{ConversationID: direct.ID, Muted: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{ConversationID: direct.ID, WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	f.events.reset()
	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-focused", SenderName: "Alex", Text: "visible in focused chat", Created: 5})
	if total, _ := f.store.UnreadTotal(); total != 0 {
		t.Errorf("focused chat unread = %d, want 0", total)
	}
	if len(f.notifier.Calls()) != 2 {
		t.Error("focused active conversation generated a notification")
	}
	if got, want := f.events.names(), []string{"message.added", "conversation.updated", "unread.changed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("focused incoming events = %v, want %v", got, want)
	}

	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{ConversationID: direct.ID, WindowActive: false}); err != nil {
		t.Fatal(err)
	}
	f.app.Incoming("wa", "remote-direct", domain.Message{RemoteID: "in-inactive", SenderName: "Alex", Text: "window inactive", Created: 6})
	if calls = f.notifier.Calls(); len(calls) != 3 || calls[2].Body != "New message" {
		t.Errorf("inactive window notification = %#v", calls)
	}
	f.app.Incoming("wa", "remote-group", domain.Message{RemoteID: "group-in", SenderName: "Priya", Text: "group text", Created: 7})
	if calls = f.notifier.Calls(); len(calls) != 4 || calls[3].Title != "Priya · Climbing Crew" || calls[3].Body != "New message" {
		t.Errorf("group notification = %#v", calls)
	}
	f.app.Incoming("wa", "not-present", domain.Message{RemoteID: "ignored", Text: "ignored"})
	if len(f.notifier.Calls()) != 4 {
		t.Error("unknown remote conversation generated a notification")
	}
}

func TestHistoryPersistsUnreadWithoutLiveNotificationsOrFocusReads(t *testing.T) {
	f := newFixture(t, false)
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	if _, err := f.app.SetFocus(context.Background(), app.FocusParams{ConversationID: conv.ID, WindowActive: true}); err != nil {
		t.Fatal(err)
	}
	f.app.History("wa", "remote-chat", domain.Message{RemoteID: "history-1", SenderName: "Alex", Text: "historical", Status: domain.StatusReceived, Created: 1})
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
	f.app.History("wa", "remote-chat", domain.Message{RemoteID: "history-1", Text: "duplicate", Status: domain.StatusReceived, Created: 2})
	if len(f.events.names()) != 0 {
		t.Error("duplicate history message emitted events")
	}
}

func TestSinkPersistsAndEmitsConnectorEvents(t *testing.T) {
	f := newFixture(t, false)
	conv := addConversation(t, f, "chat", "remote-chat", "Original", domain.KindDirect)
	f.app.AccountStatus("wa", domain.AccountConnected, "Ready")
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"account.updated"}) {
		t.Fatalf("account status events = %v", got)
	}
	f.events.reset()
	f.app.Contact(domain.Contact{AccountID: "wa", RemoteID: "contact", Name: "Contact"})
	contacts, err := f.store.Contacts("wa", "")
	if err != nil || len(contacts) != 1 {
		t.Fatalf("Contact sink did not persist contact: %#v, %v", contacts, err)
	}
	f.app.Conversation(domain.Conversation{AccountID: "wa", RemoteID: "remote-chat", Title: "Updated", Kind: domain.KindGroup, Members: 4})
	updated, err := f.store.Conversation(conv.ID)
	if err != nil || updated.Title != "Updated" || updated.Kind != domain.KindGroup || updated.Members != 4 {
		t.Fatalf("Conversation sink = %#v, %v", updated, err)
	}
	f.app.Typing("wa", "remote-chat", "Priya", true)
	if got, want := f.events.names(), []string{"conversation.updated", "typing"}; !reflect.DeepEqual(got, want) {
		t.Errorf("conversation/typing events = %v, want %v", got, want)
	}
	f.app.AccountStatus("missing", domain.AccountError, "ignored")
	f.app.Conversation(domain.Conversation{AccountID: "wa", RemoteID: "", Title: "ignored"})
	f.app.Incoming("wa", "missing", domain.Message{Text: "ignored"})
	f.app.Typing("wa", "missing", "Nobody", true)
	f.app.OutgoingStatus("missing", "remote", domain.StatusSent)

	if _, _, err := f.store.AddMessage(domain.Message{ID: "status-msg", ConversationID: conv.ID, Outgoing: true, Text: "status", Created: 1}); err != nil {
		t.Fatal(err)
	}
	f.events.reset()
	f.app.OutgoingStatus("status-msg", "remote-status", domain.StatusSent)
	updatedMessage, err := f.store.Message("status-msg")
	if err != nil || updatedMessage.Status != domain.StatusSent || updatedMessage.RemoteID != "remote-status" {
		t.Fatalf("OutgoingStatus() = %#v, %v", updatedMessage, err)
	}
	if got := f.events.names(); !reflect.DeepEqual(got, []string{"message.updated"}) {
		t.Errorf("OutgoingStatus events = %v", got)
	}
}

func TestReadMutedAndNoManagerPaths(t *testing.T) {
	f := newFixture(t, false)
	conv := addConversation(t, f, "chat", "remote-chat", "Chat", domain.KindDirect)
	for i := 0; i < 2; i++ {
		if _, _, err := f.store.AddMessage(domain.Message{ID: fmt.Sprintf("in-%d", i), ConversationID: conv.ID, Text: "incoming", Created: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.app.SetMuted(context.Background(), app.SetMutedParams{ConversationID: conv.ID, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.MarkRead(context.Background(), app.ConversationParams{ConversationID: conv.ID}); err != nil {
		t.Fatal(err)
	}
	updated, err := f.store.Conversation(conv.ID)
	if err != nil || updated.Unread != 0 {
		t.Fatalf("MarkRead() conversation = %#v, %v", updated, err)
	}
	if _, err := f.app.MarkRead(context.Background(), app.ConversationParams{ConversationID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("MarkRead(unknown) error = %v", err)
	}
	if _, err := f.app.SetMuted(context.Background(), app.SetMutedParams{ConversationID: "missing", Muted: true}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetMuted(unknown) error = %v", err)
	}

	noManager := app.New(f.store, nil, nil, f.clock)
	message, err := noManager.SendMessage(context.Background(), app.SendMessageParams{ConversationID: conv.ID, Text: "queued"})
	if !errors.Is(err, app.ErrNoManager) || message.Status != domain.StatusFailed {
		t.Errorf("SendMessage without manager = %#v, %v", message, err)
	}
	failed := addFailedMessage(t, f, conv.ID, "retry-without-manager")
	noManager.Manager = nil
	retried, err := noManager.Retry(context.Background(), app.RetryParams{MessageID: failed.ID})
	if !errors.Is(err, app.ErrNoManager) || retried.Status != domain.StatusFailed {
		t.Errorf("Retry without manager = %#v, %v", retried, err)
	}
}
