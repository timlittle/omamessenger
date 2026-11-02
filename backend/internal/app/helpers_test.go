package app_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/spell"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// fixture is an application over a real database with fake connectors,
// notifications and UI.
type fixture struct {
	store           *store.Store
	commands        *app.Commands
	ingest          *app.Ingest
	dispatcher      *fakeDispatcher
	notifier        *fakeNotifier
	published       *fakePublisher
	injector        *fakeInjector
	accounts        *fakeAccounts
	signIn          *fakeSignIn
	history         *fakeHistory
	media           *fakeMedia
	refresher       *fakeRefresher
	organizer       *fakeOrganizer
	archiveUnpinner *fakeArchiveUnpinner
	reactor         *fakeReactor
	voter           *fakeVoter
	deleter         *fakeDeleter
	editor          *fakeEditor
	blocker         *fakeBlocker
	spamReport      *fakeSpamReporter
	members         *fakeMemberLister
	limiter         *fakeLimiter
	outgoing        *cache.Outgoing
	sounds          *cache.Sounds
	clipboard       *fakeClipboard
	filePicker      *fakeFilePicker
	opener          *fakeOpener
	logger          *fakeLogger
	transcoder      *fakeTranscoder
	downloads       string
	mediaDir        string
	outgoingDir     string
	keysConfigPath  string
	transcodeRoot   string
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
	db, err := store.Open(t.Context(), dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}
	if err := db.UpsertAccount(t.Context(), account); err != nil {
		t.Fatal(err)
	}

	outgoingDir := filepath.Join(t.TempDir(), "outgoing")

	f := &fixture{
		store: db, dispatcher: &fakeDispatcher{}, notifier: &fakeNotifier{},
		published: &fakePublisher{}, injector: &fakeInjector{}, members: &fakeMemberLister{},
		accounts: &fakeAccounts{store: db}, signIn: &fakeSignIn{}, history: &fakeHistory{}, media: &fakeMedia{},
		refresher: &fakeRefresher{}, organizer: &fakeOrganizer{}, archiveUnpinner: &fakeArchiveUnpinner{unpins: map[string]bool{}}, reactor: &fakeReactor{}, voter: &fakeVoter{}, deleter: &fakeDeleter{}, editor: &fakeEditor{},
		blocker: &fakeBlocker{}, spamReport: &fakeSpamReporter{}, limiter: &fakeLimiter{limits: map[string]int64{}},
		outgoing: cache.NewOutgoing(outgoingDir, 24*time.Hour, 1<<30, storeExists(db)), clipboard: &fakeClipboard{},
		sounds:     cache.NewSounds(filepath.Join(t.TempDir(), "sounds")),
		filePicker: &fakeFilePicker{}, opener: &fakeOpener{},
		logger: &fakeLogger{}, transcoder: &fakeTranscoder{}, downloads: filepath.Join(t.TempDir(), "downloads"),
		mediaDir:       filepath.Join(t.TempDir(), "media"),
		outgoingDir:    outgoingDir,
		keysConfigPath: filepath.Join(t.TempDir(), "config", "keys.conf"),
		transcodeRoot:  filepath.Join(t.TempDir(), "work"),
	}
	if err := os.MkdirAll(f.transcodeRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	deps := app.Deps{
		Store: db, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published,
		Accounts: f.accounts, SignIn: f.signIn, History: f.history,
		Media: f.media, Cache: cache.New(f.mediaDir, 1<<20),
		Refresher: f.refresher, Organizer: f.organizer, ArchiveUnpinner: f.archiveUnpinner, Reactor: f.reactor, Voter: f.voter, Deleter: f.deleter, Editor: f.editor,
		Blocker: f.blocker, SpamReporter: f.spamReport, Members: f.members, Limiter: f.limiter,
		Outgoing: f.outgoing, Sounds: f.sounds, Clipboard: f.clipboard, FilePicker: f.filePicker, Opener: f.opener, Logger: f.logger, Spell: spell.New(dataDir, func(string) string { return "" }),
		Transcoder: f.transcoder,
		DataDir:    dataDir, DBPath: dbPath, ExecutableName: "oma-messenger-service-9.9.9", HelperVersion: "9.9.9",
		DownloadsDir:      f.downloads,
		MediaCacheDir:     f.mediaDir,
		OutgoingDir:       f.outgoingDir,
		KeysConfigPath:    f.keysConfigPath,
		TranscodeJobRoot:  f.transcodeRoot,
		PlaybackAvailable: true,
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
func appOver(t *testing.T, db *store.Store) (*app.Commands, *app.Ingest, *fakeDispatcher, *fakeNotifier) {
	t.Helper()

	dispatcher := &fakeDispatcher{}
	notifier := &fakeNotifier{}
	deps := app.Deps{
		Store: db, Dispatcher: dispatcher, Notifier: notifier, Publisher: &fakePublisher{},
		Accounts: &fakeAccounts{store: db}, SignIn: &fakeSignIn{}, History: &fakeHistory{}, Media: &fakeMedia{},
		Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20), Refresher: &fakeRefresher{},
		Organizer: &fakeOrganizer{}, ArchiveUnpinner: &fakeArchiveUnpinner{unpins: map[string]bool{}}, Reactor: &fakeReactor{}, Deleter: &fakeDeleter{}, Editor: &fakeEditor{},
		Blocker: &fakeBlocker{}, SpamReporter: &fakeSpamReporter{},
		Outgoing: cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30, storeExists(db)), Clipboard: &fakeClipboard{}, Logger: &fakeLogger{},
	}

	commands, ingest := app.New(deps)

	return commands, ingest, dispatcher, notifier
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

	// onMarkRead, when set, replaces MarkRead's own recording behaviour,
	// for a test that needs to inspect or block on the ctx a caller gave
	// it, such as one proving a debounced flush's ctx is cancelled.
	onMarkRead func(ctx context.Context, conv domain.Conversation) error
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

// sentTexts returns the text of every message given to Send so far, in
// order, safe to call while another goroutine - a scheduler running
// concurrently in a synctest test, say - may still be calling Send.
func (d *fakeDispatcher) sentTexts() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.sent)
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

func (d *fakeDispatcher) MarkRead(ctx context.Context, conv domain.Conversation) error {
	d.mu.Lock()
	onMarkRead := d.onMarkRead
	d.mu.Unlock()

	if onMarkRead != nil {
		return onMarkRead(ctx, conv)
	}

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
	return domain.Message{RemoteID: remoteID, SenderID: "alex", SenderName: "Alex", Text: text, Created: time.Now().UnixMilli()}
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

// fakeMedia plays a service's media downloads, writing the message's
// remote id as the file's content, or data instead when a test sets
// it (to drive the cache's own size limit, say), or failing with err.
type fakeMedia struct {
	fetched []string
	paths   []string
	err     error
	data    []byte
}

func (m *fakeMedia) FetchMedia(_ context.Context, _ domain.Conversation, messageRemoteID, path string, _ bool) error {
	m.fetched = append(m.fetched, messageRemoteID)
	m.paths = append(m.paths, path)
	if m.err != nil {
		return m.err
	}

	content := m.data
	if content == nil {
		content = []byte(messageRemoteID)
	}

	return os.WriteFile(path, content, 0o600)
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

// fakeClipboard answers a clipboard check from canned data, keyed by MIME
// type, in place of running wl-paste, and records every write in place
// of running wl-copy.
type fakeClipboard struct {
	types    []string
	data     map[string][]byte
	typesErr error
	readErr  error
	written  []clipboardWrite
	writeErr error
}

// clipboardWrite is one call fakeClipboard.Write recorded: the MIME
// type it was given and the bytes read off the source before the test
// goroutine's own buffer stops being valid.
type clipboardWrite struct {
	mimeType string
	data     []byte
}

// Types reports the MIME types the test set the clipboard to offer.
func (c *fakeClipboard) Types(context.Context) ([]string, error) { return c.types, c.typesErr }

// Read writes the canned data for mimeType, or fails with readErr.
func (c *fakeClipboard) Read(_ context.Context, mimeType string, w io.Writer) error {
	if c.readErr != nil {
		return c.readErr
	}

	_, err := w.Write(c.data[mimeType])

	return err
}

// ReadText returns the canned text/plain data, or fails with readErr,
// the same as Read does for an image.
func (c *fakeClipboard) ReadText(context.Context) (string, error) {
	return string(c.data["text/plain"]), c.readErr
}

// Write records mimeType and data's bytes as one clipboardWrite, or
// fails with writeErr without reading data at all.
func (c *fakeClipboard) Write(_ context.Context, mimeType string, data io.Reader) error {
	if c.writeErr != nil {
		return c.writeErr
	}

	got, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	c.written = append(c.written, clipboardWrite{mimeType: mimeType, data: got})

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
