package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func (s *Store) UpsertAccount(a domain.Account) error {
	if a.ID == "" || a.Name == "" || !domain.ValidService(a.Service) {
		return fmt.Errorf("invalid account %q", a.ID)
	}
	if a.Status == "" {
		a.Status = domain.AccountOffline
	}
	_, err := s.db.Exec(`INSERT INTO accounts(id,service,name,status,detail) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET service=excluded.service,name=excluded.name`,
		a.ID, a.Service, a.Name, a.Status, a.Detail)
	return err
}

func (s *Store) SetAccountStatus(id, status, detail string) (domain.Account, error) {
	res, err := s.db.Exec(`UPDATE accounts SET status=?,detail=? WHERE id=?`, status, detail, id)
	if err != nil {
		return domain.Account{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Account{}, domain.ErrNotFound
	}
	return s.Account(id)
}

func (s *Store) Account(id string) (domain.Account, error) {
	var a domain.Account
	err := s.db.QueryRow(`SELECT id,service,name,status,detail FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Service, &a.Name, &a.Status, &a.Detail)
	if errors.Is(err, sql.ErrNoRows) {
		return a, domain.ErrNotFound
	}
	return a, err
}

func (s *Store) Accounts() ([]domain.Account, error) {
	rows, err := s.db.Query(`SELECT id,service,name,status,detail FROM accounts ORDER BY service,name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Account{}
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.Service, &a.Name, &a.Status, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
