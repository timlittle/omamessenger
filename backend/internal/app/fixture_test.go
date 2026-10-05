package app_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
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

// fixture runs both halves of the application over a real store, with the
// connector Manager as the Commands dispatcher and Ingest as its sink.
type fixture struct {
	cmd       *app.Commands
	ingest    *app.Ingest
	config    app.Config
	manager   *connector.Manager
	store     *store.Store
	connector *testConnector
	clock     *clocktest.Clock
	notifier  *recorder
	events    *eventLog
}

type fixtureOptions struct {
	demo       bool
	setChatter func(bool)
}

func newFixture(t *testing.T, opts fixtureOptions) *fixture {
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
	f := &fixture{store: db, clock: clocktest.New(time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC)), notifier: &recorder{}, events: &eventLog{}}
	f.config = app.Config{
		Repo: db, Notifier: f.notifier, Clock: f.clock, Emit: f.events.add, Version: "test",
		Demo: opts.demo, SetChatter: opts.setChatter,
	}
	f.cmd, f.ingest = app.New(f.config)
	f.connector = &testConnector{account: domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}}
	f.manager = &connector.Manager{Store: db, Sink: f.ingest, Clock: f.clock, Connectors: []connector.Connector{f.connector}}
	f.cmd.AttachDispatcher(f.manager)
	ctx, cancel := context.WithCancel(context.Background())
	if err := f.manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		f.manager.Wait()
	})
	return f
}

// commands builds another Commands over the same store and event log with a
// modified configuration, sharing the fixture's dispatcher.
func (f *fixture) commands(modify func(*app.Config)) *app.Commands {
	cfg := f.config
	modify(&cfg)
	cmd, _ := app.New(cfg)
	cmd.AttachDispatcher(f.manager)
	return cmd
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

type testInjector struct{}

func (testInjector) Inject(remoteID string) (domain.Message, error) {
	return domain.Message{ConversationID: remoteID, Text: "injected"}, nil
}

// notification is one recorded call to recorder.Notify.
type notification struct {
	Title string
	Body  string
}

// recorder is a concurrency-safe Notifier fake.
type recorder struct {
	mu    sync.Mutex
	calls []notification
}

func (r *recorder) Notify(title, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, notification{Title: title, Body: body})
}

// Calls returns a snapshot of recorded notifications.
func (r *recorder) Calls() []notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notification(nil), r.calls...)
}
