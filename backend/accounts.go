package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// accountRegistry adds and removes the user's real accounts: it keeps
// their credentials, records them in the database and starts or stops
// their connectors, through whichever Provider handles each account's
// service.
type accountRegistry struct {
	db        *store.Store
	dataDir   string
	providers map[string]connector.Provider
	order     []string // service ids, in registration order, for Services
	manager   *connector.Manager
}

var (
	_ app.Accounts      = (*accountRegistry)(nil)
	_ app.ServiceLister = (*accountRegistry)(nil)
)

// newAccountRegistry builds a registry over db, keeping each provider's
// account files under its own subdirectory of dataDir so one service's
// files never collide with another's.
func newAccountRegistry(db *store.Store, dataDir string, providers []connector.Provider) *accountRegistry {
	r := &accountRegistry{db: db, dataDir: dataDir, providers: map[string]connector.Provider{}}
	for _, p := range providers {
		r.providers[p.Service()] = p
		r.order = append(r.order, p.Service())
	}

	return r
}

// Services lists the registered providers' services in the order they
// were given to newAccountRegistry, so the UI's chooser is stable.
func (r *accountRegistry) Services() []domain.Service {
	services := make([]domain.Service, len(r.order))
	for i, id := range r.order {
		services[i] = domain.Service{ID: id, Name: r.providers[id].Name()}
	}

	return services
}

// saved returns a connector for every account already in the database
// whose service has a registered provider, to start with the helper.
// Each shows as connecting until its connector reports, rather than
// keeping the last run's status.
func (r *accountRegistry) saved(ctx context.Context) ([]connector.Connector, error) {
	accounts, err := r.db.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	var connectors []connector.Connector
	for _, a := range accounts {
		provider, ok := r.providers[a.Service]
		if !ok {
			continue
		}

		if a, err = r.db.SetAccountStatus(ctx, a.ID, domain.AccountConnecting, ""); err != nil {
			return nil, fmt.Errorf("load accounts: %w", err)
		}

		connectors = append(connectors, provider.Connect(a, r.serviceDir(a.Service)))
	}

	return connectors, nil
}

// Add saves a new account's setup through its service's provider and
// starts its connector, which then asks the user to sign in.
func (r *accountRegistry) Add(ctx context.Context, n app.NewAccount) (domain.Account, error) {
	provider, ok := r.providers[n.Service]
	if !ok {
		return domain.Account{}, fmt.Errorf("add account: %w: %s", connector.ErrUnknownProvider, n.Service)
	}

	account := domain.Account{ID: newAccountID(n.Service), Service: n.Service, Name: provider.Name(), Status: domain.AccountConnecting}
	dir := r.serviceDir(n.Service)

	if err := provider.Prepare(dir, account.ID, n.Options); err != nil {
		return domain.Account{}, fmt.Errorf("add account: %w", err)
	}

	if err := r.db.UpsertAccount(ctx, account); err != nil {
		return domain.Account{}, errors.Join(fmt.Errorf("add account: %w", err), r.forget(ctx, account))
	}

	if err := r.manager.Add(ctx, provider.Connect(account, dir)); err != nil {
		return domain.Account{}, errors.Join(fmt.Errorf("add account: %w", err), r.forget(ctx, account))
	}

	return account, nil
}

// Remove stops an account's connector, then deletes its session,
// credentials and everything stored for it.
func (r *accountRegistry) Remove(ctx context.Context, accountID string) error {
	account, err := r.db.Account(ctx, accountID)
	if err != nil {
		return fmt.Errorf("remove account: %w", err)
	}

	if err := r.manager.Remove(ctx, accountID); err != nil && !errors.Is(err, connector.ErrNoConnector) {
		return fmt.Errorf("remove account: %w", err)
	}

	return r.forget(ctx, account)
}

// forget deletes an account's provider files, if it has a registered
// provider, and its row in the database.
func (r *accountRegistry) forget(ctx context.Context, account domain.Account) error {
	if provider, ok := r.providers[account.Service]; ok {
		if err := provider.Forget(r.serviceDir(account.Service), account.ID); err != nil {
			return fmt.Errorf("remove account files: %w", err)
		}
	}

	if err := r.db.DeleteAccount(ctx, account.ID); err != nil {
		return fmt.Errorf("remove account: %w", err)
	}

	return nil
}

// serviceDir is where a service's provider keeps every one of its
// accounts' files. Telegram's accounts predate this registry and already
// live at dataDir/telegram, which this produces unchanged since the
// Telegram service id is "telegram".
func (r *accountRegistry) serviceDir(service string) string {
	return filepath.Join(r.dataDir, service)
}

// newAccountID makes a random id for a new account of the given service,
// prefixed with it so ids stay readable at a glance; existing Telegram
// accounts keep their older "tg-" prefix.
func newAccountID(service string) string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails since Go 1.24

	return service + "-" + hex.EncodeToString(b[:])
}
