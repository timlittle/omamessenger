package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/server"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// session is a server connected to a test client over an in-memory pipe.
type session struct {
	client    *jsonrpc2.Conn
	server    *server.Server
	store     *store.Store
	events    *notifications
	ingest    *app.Ingest
	accounts  *storeAccounts
	clipboard *fakeClipboard
}

// connect serves a fresh application with one account, "wa", and a direct
// conversation, "chat". With faked set, fake.inject is available.
func connect(t *testing.T, faked bool) *session {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}

	seed(t, db)

	srv := server.New("1.2.3", log.New(io.Discard, "", 0))
	accounts := &storeAccounts{db: db}
	clipboard := &fakeClipboard{}
	deps := app.Deps{
		Store: db, Dispatcher: acceptAll{}, Notifier: silent{}, Publisher: srv, Accounts: accounts,
		SignIn: acceptAll{}, Organizer: acceptAll{}, Reactor: acceptAll{}, Deleter: acceptAll{},
		Outgoing: cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30), Clipboard: clipboard,
	}
	if faked {
		deps.Fake = unreachableFake{}
	}

	commands, ingest := app.New(deps)

	ctx, cancel := context.WithCancel(t.Context())
	serverSide, clientSide := net.Pipe()
	srv.Start(ctx, serverSide, commands)

	events := &notifications{arrived: make(chan string, 100)}
	client := jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(clientSide), events)

	t.Cleanup(func() {
		cancel()
		srv.Wait()
		_ = client.Close()
		_ = db.Close()
	})

	return &session{client: client, server: srv, store: db, events: events, ingest: ingest, accounts: accounts, clipboard: clipboard}
}

// connectWithMedia serves an application like connect, but wired with a
// MediaFetcher and MediaCache, for the one test that drives media.fetch's
// download path all the way through the server. The other sixteen
// connect callers need neither, so this stays a separate constructor
// rather than growing connect's own signature.
func connectWithMedia(t *testing.T, media app.MediaFetcher, mediaCache app.MediaCache) *session {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}

	seed(t, db)

	srv := server.New("1.2.3", log.New(io.Discard, "", 0))
	deps := app.Deps{
		Store: db, Dispatcher: acceptAll{}, Notifier: silent{}, Publisher: srv, Accounts: &storeAccounts{db: db},
		SignIn: acceptAll{}, Organizer: acceptAll{}, Reactor: acceptAll{}, Deleter: acceptAll{},
		Outgoing: cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30), Clipboard: &fakeClipboard{},
		Media: media, Cache: mediaCache,
	}

	commands, ingest := app.New(deps)

	ctx, cancel := context.WithCancel(t.Context())
	serverSide, clientSide := net.Pipe()
	srv.Start(ctx, serverSide, commands)

	events := &notifications{arrived: make(chan string, 100)}
	client := jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(clientSide), events)

	t.Cleanup(func() {
		cancel()
		srv.Wait()
		_ = client.Close()
		_ = db.Close()
	})

	return &session{client: client, server: srv, store: db, events: events, ingest: ingest}
}

// seed stores account "wa" and conversation "chat".
func seed(t *testing.T, db *store.Store) {
	t.Helper()

	ctx := t.Context()
	if err := db.UpsertAccount(ctx, domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}); err != nil {
		t.Fatal(err)
	}

	conv := domain.Conversation{ID: "chat", AccountID: "wa", RemoteID: "r-chat", Title: "Chat"}
	if _, _, err := db.EnsureConversation(ctx, conv); err != nil {
		t.Fatal(err)
	}
}

// call makes a request and decodes its result into a new R.
func call[R any](t *testing.T, s *session, method string, params any) (R, error) {
	t.Helper()

	var result R
	err := s.client.Call(t.Context(), method, params, &result)

	return result, err
}

// code returns the JSON-RPC error code of err, or 0.
func code(err error) int64 {
	var rpcErr *jsonrpc2.Error
	if !errors.As(err, &rpcErr) {
		return 0
	}

	return rpcErr.Code
}

// errorData decodes the JSON-RPC error's data field into a new T,
// reporting false when err carries none, the same way a real UI would
// read whatever safe detail, such as media.fetch's reason category,
// an error's data field carries.
func errorData[T any](err error) (T, bool) {
	var out T

	var rpcErr *jsonrpc2.Error
	if !errors.As(err, &rpcErr) || rpcErr.Data == nil {
		return out, false
	}

	return out, json.Unmarshal(*rpcErr.Data, &out) == nil
}

// fakeMediaFetcher answers FetchMedia with err, for a test that only
// cares how the server reports that failure to the UI.
type fakeMediaFetcher struct{ err error }

func (m fakeMediaFetcher) FetchMedia(context.Context, domain.Conversation, string, string) error {
	return m.err
}

// notifications passes the method of each notification the client
// receives to arrived.
type notifications struct {
	arrived chan string
}

func (n *notifications) Handle(_ context.Context, _ *jsonrpc2.Conn, req *jsonrpc2.Request) {
	n.arrived <- req.Method
}

// await waits for a notification with the given method, failing the test
// after five seconds.
func (n *notifications) await(t *testing.T, method string) {
	t.Helper()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case got := <-n.arrived:
			if got == method {
				return
			}
		case <-timeout:
			t.Fatalf("no %s notification within 5s", method)
		}
	}
}

// acceptAll is a dispatcher whose service accepts everything.
type acceptAll struct{}

func (acceptAll) Send(context.Context, domain.Conversation, domain.Message) error { return nil }
func (acceptAll) MarkRead(context.Context, domain.Conversation) error             { return nil }

// SubmitAuth accepts any sign-in answer.
func (acceptAll) SubmitAuth(context.Context, string, string, string) error { return nil }

// SetPinned and SetArchived accept any pin or archive change.
func (acceptAll) SetPinned(context.Context, domain.Conversation, bool) error   { return nil }
func (acceptAll) SetArchived(context.Context, domain.Conversation, bool) error { return nil }

// React accepts any reaction change.
func (acceptAll) React(context.Context, domain.Conversation, string, string) error { return nil }

// DeleteMessages accepts any delete.
func (acceptAll) DeleteMessages(context.Context, domain.Conversation, []string, bool) error {
	return nil
}

// storeAccounts adds and removes accounts straight in the store,
// recording the options of the last Add call so tests can check what
// reached it.
type storeAccounts struct {
	db *store.Store

	mu      sync.Mutex
	options map[string]string
}

func (a *storeAccounts) Add(ctx context.Context, n app.NewAccount) (domain.Account, error) {
	a.mu.Lock()
	a.options = n.Options
	a.mu.Unlock()

	account := domain.Account{ID: "tg-new", Service: n.Service, Name: "Telegram"}

	return account, a.db.UpsertAccount(ctx, account)
}

func (a *storeAccounts) Remove(ctx context.Context, accountID string) error {
	return a.db.DeleteAccount(ctx, accountID)
}

// Services reports one service, so tests can check hello includes it.
func (a *storeAccounts) Services() []domain.Service {
	return []domain.Service{{ID: domain.ServiceTelegram, Name: "Telegram"}}
}

// lastOptions returns the setup options of the most recent Add call.
func (a *storeAccounts) lastOptions() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.options
}

// silent is a notifier that shows nothing.
type silent struct{}

func (silent) Notify(string, string, string) {}

// fakeClipboard answers a clipboard check from canned data, keyed by MIME
// type, in place of running wl-paste.
type fakeClipboard struct {
	types    []string
	data     map[string][]byte
	typesErr error
}

func (c *fakeClipboard) Types(context.Context) ([]string, error) {
	return c.types, c.typesErr
}

func (c *fakeClipboard) Read(_ context.Context, mimeType string, w io.Writer) error {
	_, err := w.Write(c.data[mimeType])

	return err
}

// unreachableFake is an injector whose conversations are never found.
type unreachableFake struct{}

func (unreachableFake) Inject(context.Context, string) (domain.Message, error) {
	return domain.Message{}, domain.ErrNotFound
}
