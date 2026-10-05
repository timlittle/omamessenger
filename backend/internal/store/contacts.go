package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func (s *Store) UpsertContact(c domain.Contact) error {
	_, err := s.db.Exec(`INSERT INTO contacts(account_id,remote_id,name) VALUES(?,?,?)
		ON CONFLICT(account_id,remote_id) DO UPDATE SET name=excluded.name`, c.AccountID, c.RemoteID, c.Name)
	return err
}

func (s *Store) Contact(accountID, remoteID string) (domain.Contact, error) {
	c := domain.Contact{AccountID: accountID, RemoteID: remoteID}
	err := s.db.QueryRow(`SELECT name FROM contacts WHERE account_id=? AND remote_id=?`, accountID, remoteID).Scan(&c.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.ErrNotFound
	}
	return c, err
}

func (s *Store) Contacts(accountID, query string) ([]domain.Contact, error) {
	pattern := "%" + escapeLike(strings.TrimSpace(query)) + "%"
	rows, err := s.db.Query(`SELECT account_id,remote_id,name FROM contacts
		WHERE account_id=? AND name LIKE ? ESCAPE '\' ORDER BY name COLLATE NOCASE`, accountID, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Contact{}
	for rows.Next() {
		var c domain.Contact
		if err := rows.Scan(&c.AccountID, &c.RemoteID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
