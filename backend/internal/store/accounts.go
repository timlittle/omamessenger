package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// accountColumns lists the columns scanAccount reads, in order.
const accountColumns = `id,service,name,status,detail`

// ErrInvalidAccount reports an account without an id, a name or a
// supported service.
var ErrInvalidAccount = errors.New("invalid account")

// UpsertAccount records an account, or renames an existing one. The stored
// connection status is kept, because only the connector reports it.
func (s *Store) UpsertAccount(ctx context.Context, a domain.Account) error {
	if a.ID == "" || a.Name == "" || !domain.ValidService(a.Service) {
		return fmt.Errorf("store: upsert account %q: %w", a.ID, ErrInvalidAccount)
	}

	if a.Status == "" {
		a.Status = domain.AccountOffline
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO accounts(`+accountColumns+`) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET service=excluded.service,name=excluded.name`,
		a.ID, a.Service, a.Name, a.Status, a.Detail)

	return wrap("upsert account", err)
}

// SetAccountStatus records an account's connection state and returns the
// updated account.
func (s *Store) SetAccountStatus(ctx context.Context, id, status, detail string) (domain.Account, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET status=?,detail=? WHERE id=?`, status, detail, id)
	if err != nil {
		return domain.Account{}, wrap("set account status", err)
	}

	if err := requireRow("set account status", res); err != nil {
		return domain.Account{}, err
	}

	return s.Account(ctx, id)
}

// Account returns one account.
func (s *Store) Account(ctx context.Context, id string) (domain.Account, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id=?`, id)
	a, err := scanAccount(row)

	return a, wrap("account", err)
}

// Accounts returns every account, ordered by service and name.
func (s *Store) Accounts(ctx context.Context) ([]domain.Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountColumns+` FROM accounts ORDER BY service,name,id`)
	if err != nil {
		return nil, wrap("accounts", err)
	}

	accounts, err := scanAll(rows, scanAccount)

	return accounts, wrap("accounts", err)
}

// scanAccount reads one row selected with accountColumns.
func scanAccount(row scanner) (domain.Account, error) {
	var a domain.Account
	err := row.Scan(&a.ID, &a.Service, &a.Name, &a.Status, &a.Detail)

	return a, err
}

// DeleteAccount removes an account with its contacts, conversations and
// messages.
func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, id)
	if err != nil {
		return wrap("delete account", err)
	}

	return requireRow("delete account", res)
}
