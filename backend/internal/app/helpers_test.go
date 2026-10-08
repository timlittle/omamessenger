package app_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	organizer  *fakeOrganizer
	reactor    *fakeReactor
	voter      *fakeVoter
	deleter    *fakeDeleter
	members    *fakeMemberLister
	outgoing   *cache.Outgoing
	clipboard  *fakeClipboard
	logger     *fakeLogger
}

// newFixture builds an application with one WhatsApp account "wa". With
// faked set, it has fake connectors that can inject messages.
func newFixture(t *testing.T, faked bool) *fixture {
	t.Helper()

	dataDir := t.TempDir()
	if err := os.Chmod(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dataDir, "messages.db")
	db, err := store.Open(t.Context(), dbPath)
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
		refresher: &fakeRefresher{}, organizer: &fakeOrganizer{}, reactor: &fakeReactor{}, voter: &fakeVoter{}, deleter: &fakeDeleter{},
		members: &fakeMemberLister{}, outgoing: cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing")), clipboard: &fakeClipboard{},
		logger: &fakeLogger{},
	}

	deps := app.Deps{
		Store: db, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published,
		Accounts: f.accounts, SignIn: f.signIn, History: f.history,
		Media: f.media, Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20),
		Refresher: f.refresher, Organizer: f.organizer, Reactor: f.reactor, Voter: f.voter, Deleter: f.deleter, Members: f.members,
		Outgoing: f.outgoing, Clipboard: f.clipboard, Logger: f.logger,
		DataDir: dataDir, DBPath: dbPath, ExecutableName: "oma-messenger-service-9.9.9", HelperVersion: "9.9.9",
	}
	if faked {
		deps.Fake = f.injector
	}

	f.commands, f.ingest = app.New(deps)
	f.history.ingest = f.ingest
	f.refresher.ingest = f.ingest

	return f
}

// appOver builds a Commands and Ingest pair over an already-open store,
// with its own fresh set of fake connectors, for a test that opens the
// store itself, such as one simulating a helper restart over the same
// on-disk database.
func appOver(t *testing.T, db *store.Store) (*app.Commands, *app.Ingest, *fakeDispatcher) {
	t.Helper()

	dispatcher := &fakeDispatcher{}
	deps := app.Deps{
		Store: db, Dispatcher: dispatcher, Notifier: &fakeNotifier{}, Publisher: &fakePublisher{},
		Accounts: &fakeAccounts{store: db}, SignIn: &fakeSignIn{}, History: &fakeHistory{}, Media: &fakeMedia{},
		Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20), Refresher: &fakeRefresher{},
		Organizer: &fakeOrganizer{}, Reactor: &fakeReactor{}, Deleter: &fakeDeleter{},
		Outgoing: cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing")), Clipboard: &fakeClipboard{}, Logger: &fakeLogger{},
	}

	commands, ingest := app.New(deps)

	return commands, ingest, dispatcher
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
	mu       sync.Mutex
	sent     []string
	messages []domain.Message // the full message of each Send, in order
	read     []string
	readConv []domain.Conversation // the full conversation of each MarkRead call, in order
	err      error
	onRun    func(domain.Message) // called during Send, like a fast service
}

func (d *fakeDispatcher) Send(_ context.Context, _ domain.Conversation, m domain.Message) error {
	d.mu.Lock()
	d.sent = append(d.sent, m.Text)
	d.messages = append(d.messages, m)
	onRun, err := d.onRun, d.err
	d.mu.Unlock()

	if onRun != nil {
		onRun(m)
	}

	return err
}

// last returns the most recent message given to Send.
func (d *fakeDispatcher) last() domain.Message {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.messages[len(d.messages)-1]
}

// lastMedia returns the media of the most recently dispatched message, or
// nil if it had none or nothing was dispatched.
func (d *fakeDispatcher) lastMedia() *domain.Media {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.messages) == 0 {
		return nil
	}

	return d.messages[len(d.messages)-1].Media
}

func (d *fakeDispatcher) MarkRead(_ context.Context, conv domain.Conversation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.read = append(d.read, conv.ID)
	d.readConv = append(d.readConv, conv)

	return nil
}

// fakeNotifier records notifications as "title: body" and the conversation
// id each one carried.
type fakeNotifier struct {
	mu    sync.Mutex
	shown []string
	convs []string
}

func (n *fakeNotifier) Notify(title, body, conversationID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.shown = append(n.shown, title+": "+body)
	n.convs = append(n.convs, conversationID)
}

// all returns the notifications shown so far.
func (n *fakeNotifier) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return slices.Clone(n.shown)
}

// conversations returns the conversation id carried by each notification
// shown so far, in order.
func (n *fakeNotifier) conversations() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return slices.Clone(n.convs)
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
	// duringLoad, when set, runs after the messages are reported but
	// before LoadOlder returns, standing in for a request that lands on
	// another connection while this fetch is still in flight.
	duringLoad func()
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

	if h.duringLoad != nil {
		h.duringLoad()
	}

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

// fakeCache fails every Fetch with err, without ever calling fill, for
// testing how FetchMedia reports a failure the cache itself caused
// rather than one the connector reported.
type fakeCache struct{ err error }

func (c *fakeCache) Fetch(context.Context, string, func(context.Context, string) error) (string, error) {
	return "", c.err
}

// fakeLogger records every diagnostic line FetchMedia writes, so a test
// can check the safe reason category reached it without a real
// *log.Logger and its own writer to parse.
type fakeLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *fakeLogger) Printf(format string, v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, fmt.Sprintf(format, v...))
}

// take returns the logged lines so far and forgets them.
func (l *fakeLogger) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	lines := l.lines
	l.lines = nil

	return lines
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

// fakeOrganizer records the pins and archives it is asked for, as
// "<conversationId> <value>", failing with err when it is set.
type fakeOrganizer struct {
	mu       sync.Mutex
	pinned   []string
	archived []string
	err      error
}

func (o *fakeOrganizer) SetPinned(_ context.Context, conv domain.Conversation, pinned bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.pinned = append(o.pinned, fmt.Sprintf("%s %t", conv.ID, pinned))

	return o.err
}

func (o *fakeOrganizer) SetArchived(_ context.Context, conv domain.Conversation, archived bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.archived = append(o.archived, fmt.Sprintf("%s %t", conv.ID, archived))

	return o.err
}

// fakeMemberLister reports a scripted member list, failing with err
// when it is set.
type fakeMemberLister struct {
	mu      sync.Mutex
	members []domain.Member
	err     error
	asked   []string
}

func (l *fakeMemberLister) Members(_ context.Context, conv domain.Conversation) ([]domain.Member, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.asked = append(l.asked, conv.ID)

	return l.members, l.err
}

// fakeReactor records the reactions it is asked for, as
// "<messageRemoteId> <emoji>", failing with err when it is set.
type fakeReactor struct {
	mu      sync.Mutex
	reacted []string
	err     error
}

func (r *fakeReactor) React(_ context.Context, _ domain.Conversation, messageRemoteID, emoji string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reacted = append(r.reacted, messageRemoteID+" "+emoji)

	return r.err
}

// fakeVoter records the votes it is asked to cast, as "messageRemoteID
// optionIDs", failing with err when set.
type fakeVoter struct {
	mu    sync.Mutex
	voted []string
	err   error
}

func (v *fakeVoter) Vote(_ context.Context, _ domain.Conversation, messageRemoteID string, optionIDs []string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.voted = append(v.voted, fmt.Sprintf("%s %v", messageRemoteID, optionIDs))

	return v.err
}

// fakeDeleter records the deletes it is asked for, as
// "<conversationId> <remoteIds> <forEveryone>", failing with err when
// it is set.
type fakeDeleter struct {
	mu      sync.Mutex
	deleted []string
	err     error
}

func (d *fakeDeleter) DeleteMessages(_ context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.deleted = append(d.deleted, fmt.Sprintf("%s %s %t", conv.ID, strings.Join(remoteIDs, ","), forEveryone))

	return d.err
}

// fakeClipboard answers a clipboard check from canned data, keyed by MIME
// type, in place of running wl-paste.
type fakeClipboard struct {
	types    []string
	data     map[string][]byte
	typesErr error
	readErr  error
}

// Types reports the MIME types the test set the clipboard to offer.
func (c *fakeClipboard) Types(context.Context) ([]string, error) {
	return c.types, c.typesErr
}

// Read writes the canned data for mimeType, or fails with readErr.
func (c *fakeClipboard) Read(_ context.Context, mimeType string, w io.Writer) error {
	if c.readErr != nil {
		return c.readErr
	}

	_, err := w.Write(c.data[mimeType])

	return err
}

// fakeSignIn records the sign-in answers it is given.
type fakeSignIn struct {
	answers []string
}

func (s *fakeSignIn) SubmitAuth(_ context.Context, accountID, step, value string) error {
	s.answers = append(s.answers, accountID+" "+step+"="+value)

	return nil
}
