package app_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
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
	accounts   *fakeAccounts
	signIn     *fakeSignIn
	history    *fakeHistory
	media      *fakeMedia
	refresher  *fakeRefresher
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
		accounts: &fakeAccounts{store: db}, signIn: &fakeSignIn{}, history: &fakeHistory{}, media: &fakeMedia{},
		refresher: &fakeRefresher{},
	}

	deps := app.Deps{
		Store: db, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published,
		Accounts: f.accounts, SignIn: f.signIn, History: f.history,
		Media: f.media, Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20),
		Refresher: f.refresher,
	}
	if faked {
		deps.Fake = f.injector
	}

	f.commands, f.ingest = app.New(deps)
	f.history.ingest = f.ingest
	f.refresher.ingest = f.ingest

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

// lastOf returns the data of the most recent event named name, or nil.
func (p *fakePublisher) lastOf(name string) any {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := len(p.names) - 1; i >= 0; i-- {
		if p.names[i] == name {
			return p.data[i]
		}
	}

	return nil
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

// fakeAccounts adds and removes accounts straight in the store, failing
// with err when it is set.
type fakeAccounts struct {
	store    *store.Store
	added    []app.NewAccount
	services []domain.Service
	err      error
}

func (a *fakeAccounts) Add(ctx context.Context, n app.NewAccount) (domain.Account, error) {
	if a.err != nil {
		return domain.Account{}, a.err
	}

	a.added = append(a.added, n)
	account := domain.Account{ID: "tg-new", Service: n.Service, Name: "Telegram", Status: "needs-auth"}

	return account, a.store.UpsertAccount(ctx, account)
}

func (a *fakeAccounts) Remove(ctx context.Context, accountID string) error {
	if a.err != nil {
		return a.err
	}

	return a.store.DeleteAccount(ctx, accountID)
}

// Services lists the services fakeAccounts was told to offer, so tests can
// exercise Commands.Services without a real registry.
func (a *fakeAccounts) Services() []domain.Service {
	return a.services
}

// noListerAccounts is an Accounts that does not implement
// app.ServiceLister, for testing that Commands.Services tolerates that.
type noListerAccounts struct{}

func (noListerAccounts) Add(context.Context, app.NewAccount) (domain.Account, error) {
	return domain.Account{}, nil
}

func (noListerAccounts) Remove(context.Context, string) error { return nil }

// fakeHistory plays the service's older history: each LoadOlder reports
// the next of older through the ingest, as a connector does through its
// Sink, and records where it was asked to load from.
type fakeHistory struct {
	ingest *app.Ingest
	older  []domain.Message
	from   []string
	err    error
}

func (h *fakeHistory) LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error) {
	h.from = append(h.from, beforeRemoteID)
	if h.err != nil {
		return 0, h.err
	}

	n := min(limit, len(h.older))
	for _, m := range h.older[:n] {
		h.ingest.History(ctx, conv.AccountID, conv.RemoteID, m)
	}
	h.older = h.older[n:]

	return n, nil
}

// fakeMedia plays a service's media downloads, writing the message's remote
// id as the file's content, or failing with err.
type fakeMedia struct {
	fetched []string
	err     error
}

func (m *fakeMedia) FetchMedia(_ context.Context, _ domain.Conversation, messageRemoteID, path string) error {
	m.fetched = append(m.fetched, messageRemoteID)
	if m.err != nil {
		return m.err
	}

	return os.WriteFile(path, []byte(messageRemoteID), 0o600)
}

// fakeRefresher plays a service's response to being asked to re-report
// messages: for each remote id it is given a message to report, it reports
// that message through the ingest, as a connector does through its Sink.
// It records every batch it was asked to refresh.
type fakeRefresher struct {
	ingest *app.Ingest
	toSend map[string]domain.Message
	asked  [][]string
	err    error
}

func (r *fakeRefresher) RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error {
	r.asked = append(r.asked, remoteIDs)
	if r.err != nil {
		return r.err
	}

	for _, id := range remoteIDs {
		if m, ok := r.toSend[id]; ok {
			r.ingest.History(ctx, conv.AccountID, conv.RemoteID, m)
		}
	}

	return nil
}

// fakeSignIn records the sign-in answers it is given.
type fakeSignIn struct {
	answers []string
}

func (s *fakeSignIn) SubmitAuth(_ context.Context, accountID, step, value string) error {
	s.answers = append(s.answers, accountID+" "+step+"="+value)

	return nil
}
