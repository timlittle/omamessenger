package telegram

// realIngest wires a real app.Ingest to a real SQLite store, for a test
// that proves a message the connector reports is actually kept by the
// store - not just recorded by connectortest.Sink's own simplified
// bookkeeping, which is how the Signal titling bug escaped testing (see
// EnsureConversation's own title check in backend/internal/store).
// Split out of helpers_test.go to keep neither file past the project's
// own file-length limit.

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// noopNotifier discards every notification, for a test driving a real
// app.Ingest that only cares what ends up in the store.
type noopNotifier struct{}

func (noopNotifier) Notify(string, string, string) {}

// noopPublisher discards every event, for the same reason.
type noopPublisher struct{}

func (noopPublisher) Publish(context.Context, string, any) {}

// noopDispatcher never sends or marks read, for a test that only
// drives incoming updates.
type noopDispatcher struct{}

func (noopDispatcher) Send(context.Context, domain.Conversation, domain.Message) error { return nil }

func (noopDispatcher) MarkRead(context.Context, domain.Conversation) error { return nil }

// testLogger records logged lines, so a test can confirm a drop was
// logged rather than silent.
type testLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *testLogger) Printf(format string, v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, fmt.Sprintf(format, v...))
}

// take returns the logged lines so far and forgets them.
func (l *testLogger) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	lines := l.lines
	l.lines = nil

	return lines
}

// realAccountID is the account every realIngest fixture registers in
// its store, matching the account id a connector built for one of
// these tests reports under.
const realAccountID = "tg"

// realIngest is a real app.Ingest wired to a fresh SQLite store.
type realIngest struct {
	*app.Ingest
	store  *store.Store
	logger *testLogger
}

// newRealIngest opens a fresh store in a temp directory, registers the
// realAccountID Telegram account, and wires a real app.Ingest to it.
func newRealIngest(t *testing.T) *realIngest {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "messages.db")
	db, err := store.Open(t.Context(), dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.UpsertAccount(t.Context(), domain.Account{ID: realAccountID, Service: domain.ServiceTelegram, Name: "Personal"}); err != nil {
		t.Fatal(err)
	}

	logger := &testLogger{}
	_, ingest := app.New(app.Deps{
		Store: db, Notifier: noopNotifier{}, Publisher: noopPublisher{}, Dispatcher: noopDispatcher{}, Logger: logger,
	})

	return &realIngest{Ingest: ingest, store: db, logger: logger}
}

// realConnector returns a connector for realAccountID, with no session
// database and no network, for a test that drives one of this
// package's unexported handlers directly against a real app.Ingest.
func realConnector() *Connector {
	return New(domain.Account{ID: realAccountID, Service: domain.ServiceTelegram}, "")
}
