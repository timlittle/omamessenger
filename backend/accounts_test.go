package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// idleConnector stands in for a real connector: it runs until stopped and
// says when it has started.
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

// fakeProvider stands in for a messaging service's provider: it records
// the accounts it is asked to prepare or forget, and its connector is an
// idleConnector reporting on started. Connectors take the account id
// from idOf when it is set, so a test can force a duplicate.
type fakeProvider struct {
	service, name string
	idOf          func(domain.Account) string
	started       chan string
	prepareErr    error

	prepared  []string
	forgotten []string
}

// Service reports the service id this fake handles.
func (p *fakeProvider) Service() string { return p.service }

// Name is the fake's display name.
func (p *fakeProvider) Name() string { return p.name }

// Prepare records the account it was asked to prepare, failing with
// prepareErr when it is set.
func (p *fakeProvider) Prepare(_, accountID string, _ map[string]string) error {
	p.prepared = append(p.prepared, accountID)

	return p.prepareErr
}

// Connect returns an idleConnector for the account, remapping its id
// through idOf when one is given.
func (p *fakeProvider) Connect(account domain.Account, _ string) connector.Connector {
	if p.idOf != nil {
		account.ID = p.idOf(account)
	}

	return idleConnector{account: account, started: p.started}
}

// Forget records the account it was asked to forget.
func (p *fakeProvider) Forget(_, accountID string) error {
	p.forgotten = append(p.forgotten, accountID)

	return nil
}

// testRegistry returns a registry over a fresh database, with a fake
// Telegram provider, its manager running until the test ends, and the
// channel its connectors report starting on.
func testRegistry(t *testing.T, idOf func(domain.Account) string, saved ...domain.Account) (*accountRegistry, *fakeProvider) {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() }) // nothing to report once the test is over

	for _, a := range saved {
		if err := db.UpsertAccount(t.Context(), a); err != nil {
			t.Fatal(err)
		}
	}

	provider := &fakeProvider{service: domain.ServiceTelegram, name: "Telegram", idOf: idOf, started: make(chan string, 4)}
	r := newAccountRegistry(db, t.TempDir(), []connector.Provider{provider})

	return r, provider
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

func TestAccountRegistry_AddsAndRemovesAnAccount(t *testing.T) {
	t.Parallel()

	r, provider := testRegistry(t, nil)
	startManager(t, r)

	account, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, Options: map[string]string{"apiId": "1", "apiHash": "hash"}})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(account.ID, "telegram-") {
		t.Errorf("account id %q, want a telegram- prefix", account.ID)
	}

	if id := <-provider.started; id != account.ID {
		t.Errorf("started %q, want %q", id, account.ID)
	}

	if !slices.Equal(provider.prepared, []string{account.ID}) {
		t.Errorf("prepared %v, want [%s]", provider.prepared, account.ID)
	}

	if err := r.Remove(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(provider.forgotten, []string{account.ID}) {
		t.Errorf("forgotten %v, want [%s]", provider.forgotten, account.ID)
	}

	if _, err := r.db.Account(t.Context(), account.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("account kept after removal: %v", err)
	}
}

func TestAccountRegistry_StartsSavedAccountsWithARegisteredProviderOnly(t *testing.T) {
	t.Parallel()

	r, provider := testRegistry(t, nil,
		domain.Account{ID: "tg-1", Service: domain.ServiceTelegram, Name: "Telegram", Status: domain.AccountError},
		domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp, Name: "WhatsApp", Status: domain.AccountOffline},
	)
	startManager(t, r)

	if id := <-provider.started; id != "tg-1" {
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

func TestAccountRegistry_AddRejectsAnUnregisteredService(t *testing.T) {
	t.Parallel()

	r, _ := testRegistry(t, nil)
	startManager(t, r)

	if _, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceWhatsApp}); !errors.Is(err, connector.ErrUnknownProvider) {
		t.Errorf("Add = %v, want connector.ErrUnknownProvider", err)
	}
}

func TestAccountRegistry_AddCleansUpWhenTheConnectorCannotStart(t *testing.T) {
	t.Parallel()

	// Every connector claims the saved account's id, so the new one is a
	// duplicate the manager refuses.
	saved := domain.Account{ID: "tg-1", Service: domain.ServiceTelegram, Name: "Telegram", Status: domain.AccountConnecting}
	r, provider := testRegistry(t, func(domain.Account) string { return "tg-1" }, saved)
	startManager(t, r)
	<-provider.started

	_, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram})
	if !errors.Is(err, connector.ErrDuplicateAccount) {
		t.Fatalf("Add = %v, want connector.ErrDuplicateAccount", err)
	}

	accounts, err := r.db.Accounts(t.Context())
	if err != nil || len(accounts) != 1 {
		t.Errorf("accounts = %+v, %v; want only the saved one", accounts, err)
	}

	if len(provider.forgotten) != 1 {
		t.Errorf("forgotten = %v, want the new account cleaned up", provider.forgotten)
	}
}

func TestAccountRegistry_AddCleansUpWhenTheDatabaseFails(t *testing.T) {
	t.Parallel()

	r, provider := testRegistry(t, nil)
	startManager(t, r)
	if err := r.db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram}); err == nil {
		t.Error("Add succeeded with the database closed")
	}

	if len(provider.forgotten) != 1 {
		t.Errorf("forgotten = %v, want the account cleaned up", provider.forgotten)
	}
}

func TestAccountRegistry_AddFailsWhenPrepareDoes(t *testing.T) {
	t.Parallel()

	r, provider := testRegistry(t, nil)
	startManager(t, r)
	provider.prepareErr = errors.New("telegram: bad setup")

	if _, err := r.Add(t.Context(), app.NewAccount{Service: domain.ServiceTelegram}); !errors.Is(err, provider.prepareErr) {
		t.Errorf("Add = %v, want %v", err, provider.prepareErr)
	}

	if accounts, err := r.db.Accounts(t.Context()); err != nil || len(accounts) != 0 {
		t.Errorf("accounts = %+v, %v; want none saved", accounts, err)
	}
}

func TestAccountRegistry_ServicesListsRegisteredProvidersInOrder(t *testing.T) {
	t.Parallel()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	telegram := &fakeProvider{service: domain.ServiceTelegram, name: "Telegram"}
	whatsapp := &fakeProvider{service: domain.ServiceWhatsApp, name: "WhatsApp"}
	r := newAccountRegistry(db, t.TempDir(), []connector.Provider{telegram, whatsapp})

	want := []domain.Service{{ID: domain.ServiceTelegram, Name: "Telegram"}, {ID: domain.ServiceWhatsApp, Name: "WhatsApp"}}
	if got := r.Services(); !slices.Equal(got, want) {
		t.Errorf("Services() = %+v, want %+v", got, want)
	}
}
