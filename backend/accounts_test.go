package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// idleConnector stands in for Telegram: it runs until stopped and says
// when it has started.
type idleConnector struct {
	account domain.Account
	started chan string
}

// Account describes the stand-in account.
func (c idleConnector) Account() domain.Account { return c.account }

// Run reports the start and waits to be stopped.
func (c idleConnector) Run(ctx context.Context, _ connector.Sink) error {
	c.started <- c.account.ID
	<-ctx.Done()

	return ctx.Err()
}

// Send accepts every message.
func (idleConnector) Send(context.Context, domain.Conversation, domain.Message) error { return nil }

// MarkRead accepts every read.
func (idleConnector) MarkRead(context.Context, domain.Conversation) error { return nil }

// quietSink ignores what connectors report.
type quietSink struct{ connector.Sink }

// AccountStatus ignores status changes.
func (quietSink) AccountStatus(context.Context, string, string, string) {}

// testRegistry returns a registry over a fresh database, its manager
// running until the test ends, and the channel its connectors report
// starting on. Connectors take the account id from idOf when it is set.
func testRegistry(t *testing.T, idOf func(domain.Account) string, saved ...domain.Account) (*accountRegistry, chan string) {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() }) // nothing to report once the test is over

	for _, a := range saved {
		if err := db.UpsertAccount(t.Context(), a); err != nil {
			t.Fatal(err)
		}
	}

	started := make(chan string, 4)
	r := &accountRegistry{db: db, dir: filepath.Join(t.TempDir(), "telegram")}
	r.connect = func(a domain.Account, _ string) connector.Connector {
		if idOf != nil {
			a.ID = idOf(a)
		}

		return idleConnector{account: a, started: started}
	}

	return r, started
}

// startManager runs the registry's saved accounts until the test ends.
func startManager(t *testing.T, r *accountRegistry) {
	t.Helper()

	connectors, err := r.saved(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if r.manager, err = connector.NewManager(connectors...); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); r.manager.Wait() })

	if err := r.manager.Start(ctx, r.db, quietSink{}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountRegistry_AddsAndRemovesATelegramAccount(t *testing.T) {
	t.Parallel()

	r, started := testRegistry(t, nil)
	startManager(t, r)

	account, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, APIID: 1, APIHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}

	if id := <-started; id != account.ID {
		t.Errorf("started %q, want %q", id, account.ID)
	}

	creds := filepath.Join(r.dir, account.ID+".json")
	if info, err := os.Stat(creds); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("credentials file: %v, %v; want private", info, err)
	}

	if err := r.Remove(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(creds); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("credentials kept after removal: %v", err)
	}

	if _, err := r.db.Account(t.Context(), account.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("account kept after removal: %v", err)
	}
}

func TestAccountRegistry_StartsSavedTelegramAccountsOnly(t *testing.T) {
	t.Parallel()

	r, started := testRegistry(t, nil,
		domain.Account{ID: "tg-1", Service: domain.ServiceTelegram, Name: "Telegram", Status: domain.AccountError},
		domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp, Name: "WhatsApp", Status: domain.AccountOffline},
	)
	startManager(t, r)

	if id := <-started; id != "tg-1" {
		t.Errorf("started %q, want tg-1", id)
	}

	account, err := r.db.Account(t.Context(), "tg-1")
	if err != nil || account.Status != domain.AccountConnecting {
		t.Errorf("saved account = %+v, %v; want connecting again", account, err)
	}
}

func TestAccountRegistry_RemoveRejectsUnknownAccounts(t *testing.T) {
	t.Parallel()

	r, _ := testRegistry(t, nil)
	startManager(t, r)

	if err := r.Remove(t.Context(), "tg-missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Remove = %v, want domain.ErrNotFound", err)
	}
}

func TestAccountRegistry_AddCleansUpWhenTheConnectorCannotStart(t *testing.T) {
	t.Parallel()

	// Every connector claims the saved account's id, so the new one is a
	// duplicate the manager refuses.
	saved := domain.Account{ID: "tg-1", Service: domain.ServiceTelegram, Name: "Telegram", Status: domain.AccountConnecting}
	r, started := testRegistry(t, func(domain.Account) string { return "tg-1" }, saved)
	startManager(t, r)
	<-started

	_, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, APIID: 1, APIHash: "hash"})
	if !errors.Is(err, connector.ErrDuplicateAccount) {
		t.Fatalf("Add = %v, want connector.ErrDuplicateAccount", err)
	}

	accounts, err := r.db.Accounts(t.Context())
	if err != nil || len(accounts) != 1 {
		t.Errorf("accounts = %+v, %v; want only the saved one", accounts, err)
	}

	if files, _ := os.ReadDir(r.dir); len(files) != 0 { // a missing directory reads as empty too
		t.Errorf("files left behind: %v", files)
	}
}

func TestAccountRegistry_AddFailsWithoutTheDatabase(t *testing.T) {
	t.Parallel()

	r, _ := testRegistry(t, nil)
	startManager(t, r)
	if err := r.db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, APIID: 1, APIHash: "hash"}); err == nil {
		t.Error("Add succeeded with the database closed")
	}

	if files, _ := os.ReadDir(r.dir); len(files) != 0 { // a missing directory reads as empty too
		t.Errorf("credentials left behind: %v", files)
	}
}
