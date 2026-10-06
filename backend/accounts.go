package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/telegram"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// accountRegistry adds and removes the user's real accounts: it keeps
// their credentials, records them in the database and starts or stops
// their connectors.
type accountRegistry struct {
	db      *store.Store
	dir     string
	connect func(account domain.Account, dir string) connector.Connector
	manager *connector.Manager
}

var _ app.Accounts = (*accountRegistry)(nil)

// saved returns a connector for every Telegram account already in the
// database, to start with the helper. Each shows as connecting until its
// connector reports, rather than keeping the last run's status.
func (r *accountRegistry) saved(ctx context.Context) ([]connector.Connector, error) {
	accounts, err := r.db.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	var connectors []connector.Connector
	for _, a := range accounts {
		if a.Service != domain.ServiceTelegram {
			continue
		}

		if a, err = r.db.SetAccountStatus(ctx, a.ID, domain.AccountConnecting, ""); err != nil {
			return nil, fmt.Errorf("load accounts: %w", err)
		}

		connectors = append(connectors, r.connect(a, r.dir))
	}

	return connectors, nil
}

// Add saves a new Telegram account's API credentials, OmaMessenger's own
// unless the user gave theirs, and starts its connector, which then asks
// the user to sign in.
func (r *accountRegistry) Add(ctx context.Context, n app.NewAccount) (domain.Account, error) {
	account := domain.Account{ID: newAccountID(), Service: domain.ServiceTelegram, Name: "Telegram", Status: domain.AccountConnecting}

	creds := telegram.AppCredentials()
	if n.APIID != 0 {
		creds = telegram.Credentials{APIID: n.APIID, APIHash: n.APIHash}
	}
	if err := telegram.SaveCredentials(r.dir, account.ID, creds); err != nil {
		return domain.Account{}, fmt.Errorf("add account: %w", err)
	}

	if err := r.db.UpsertAccount(ctx, account); err != nil {
		return domain.Account{}, errors.Join(fmt.Errorf("add account: %w", err), telegram.Forget(r.dir, account.ID))
	}

	if err := r.manager.Add(ctx, r.connect(account, r.dir)); err != nil {
		return domain.Account{}, errors.Join(fmt.Errorf("add account: %w", err), r.forget(ctx, account.ID))
	}

	return account, nil
}

// Remove stops an account's connector, then deletes its session,
// credentials and everything stored for it.
func (r *accountRegistry) Remove(ctx context.Context, accountID string) error {
	if _, err := r.db.Account(ctx, accountID); err != nil {
		return fmt.Errorf("remove account: %w", err)
	}

	if err := r.manager.Remove(ctx, accountID); err != nil && !errors.Is(err, connector.ErrNoConnector) {
		return fmt.Errorf("remove account: %w", err)
	}

	return r.forget(ctx, accountID)
}

// forget deletes an account's files and its rows in the database.
func (r *accountRegistry) forget(ctx context.Context, accountID string) error {
	if err := telegram.Forget(r.dir, accountID); err != nil {
		return fmt.Errorf("remove account files: %w", err)
	}

	if err := r.db.DeleteAccount(ctx, accountID); err != nil {
		return fmt.Errorf("remove account: %w", err)
	}

	return nil
}

// newAccountID makes a random id for a new Telegram account.
func newAccountID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails since Go 1.24

	return "tg-" + hex.EncodeToString(b[:])
}

// telegramConnector starts the real Telegram connector for an account.
func telegramConnector(account domain.Account, dir string) connector.Connector {
	return telegram.New(account, dir)
}
