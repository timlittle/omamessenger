package app_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// fixture is an application over a real database with fake connectors,
// notifications and UI.
type fixture struct {
	store      *store.Store
	commands   *app.Commands
	ingest     *app.Ingest
	dispatcher *fakeDispatcher
	notifier   *fakeNotifier
	published  *fakePublisher
	injector   *fakeInjector
}

// newFixture builds an application with one WhatsApp account "wa". With
// faked set, it has fake connectors that can inject messages.
func newFixture(t *testing.T, faked bool) *fixture {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}
	if err := db.UpsertAccount(t.Context(), account); err != nil {
		t.Fatal(err)
	}

	f := &fixture{
		store: db, dispatcher: &fakeDispatcher{}, notifier: &fakeNotifier{},
		published: &fakePublisher{}, injector: &fakeInjector{},
	}

	deps := app.Deps{Store: db, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published}
	if faked {
		deps.Fake = f.injector
	}

	f.commands, f.ingest = app.New(deps)

	return f
}

// conversation stores a conversation of the given kind with remote id
// "r-" + id, and forgets the events so far.
func (f *fixture) conversation(t *testing.T, id, title, kind string) domain.Conversation {
	t.Helper()

	c, _, err := f.store.EnsureConversation(t.Context(), domain.Conversation{
		ID: id, AccountID: "wa", RemoteID: "r-" + id, Title: title, Kind: kind,
	})
	if err != nil {
		t.Fatal(err)
	}

	f.published.take()

	return c
}

// fakeDispatcher records sends and read receipts, returning err for sends.
type fakeDispatcher struct {
	mu    sync.Mutex
	sent  []string
	read  []string
	err   error
	onRun func(domain.Message) // called during Send, like a fast service
}

func (d *fakeDispatcher) Send(_ context.Context, _ domain.Conversation, m domain.Message) error {
	d.mu.Lock()
	d.sent = append(d.sent, m.Text)
	onRun, err := d.onRun, d.err
	d.mu.Unlock()

	if onRun != nil {
		onRun(m)
	}

	return err
}

func (d *fakeDispatcher) MarkRead(_ context.Context, conv domain.Conversation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.read = append(d.read, conv.ID)

	return nil
}

// fakeNotifier records notifications as "title: body".
type fakeNotifier struct {
	mu    sync.Mutex
	shown []string
}

func (n *fakeNotifier) Notify(title, body string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.shown = append(n.shown, title+": "+body)
}

// all returns the notifications shown so far.
func (n *fakeNotifier) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return slices.Clone(n.shown)
}

// fakePublisher records published event names and data.
type fakePublisher struct {
	mu    sync.Mutex
	names []string
	data  []any
}

func (p *fakePublisher) Publish(_ context.Context, event string, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.names = append(p.names, event)
	p.data = append(p.data, data)
}

// take returns the event names published so far and forgets them.
func (p *fakePublisher) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	names := p.names
	p.names, p.data = nil, nil

	return names
}

// last returns the data of the most recent event.
func (p *fakePublisher) last() any {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.data[len(p.data)-1]
}

// fakeInjector injects through whatever function the test sets.
type fakeInjector struct {
	inject func(ctx context.Context, remoteID string) (domain.Message, error)
}

func (i *fakeInjector) Inject(ctx context.Context, remoteID string) (domain.Message, error) {
	return i.inject(ctx, remoteID)
}

// incoming returns an incoming message from Alex with the given remote id.
func incoming(remoteID, text string) domain.Message {
	return domain.Message{RemoteID: remoteID, SenderID: "alex", SenderName: "Alex", Text: text, Created: 1}
}
