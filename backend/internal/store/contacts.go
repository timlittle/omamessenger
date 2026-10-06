package store

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// contactColumns lists the columns scanContact reads, in order.
const contactColumns = `account_id,remote_id,name`

// UpsertContact records a contact, or renames an existing one.
func (s *Store) UpsertContact(ctx context.Context, c domain.Contact) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO contacts(`+contactColumns+`) VALUES(?,?,?)
		ON CONFLICT(account_id,remote_id) DO UPDATE SET name=excluded.name`,
		c.AccountID, c.RemoteID, c.Name)

	return wrap("upsert contact", err)
}

// Contact returns one contact of an account.
func (s *Store) Contact(ctx context.Context, accountID, remoteID string) (domain.Contact, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+contactColumns+` FROM contacts
		WHERE account_id=? AND remote_id=?`, accountID, remoteID)
	c, err := scanContact(row)

	return c, wrap("contact", err)
}

// Contacts returns an account's contacts whose name contains query, ignoring
// case, ordered by name.
func (s *Store) Contacts(ctx context.Context, accountID, query string) ([]domain.Contact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+contactColumns+` FROM contacts
		WHERE account_id=? AND name LIKE ? ESCAPE '\' ORDER BY name COLLATE NOCASE`,
		accountID, likePattern(query))
	if err != nil {
		return nil, wrap("contacts", err)
	}

	contacts, err := scanAll(rows, scanContact)

	return contacts, wrap("contacts", err)
}

// scanContact reads one row selected with contactColumns.
func scanContact(row scanner) (domain.Contact, error) {
	var c domain.Contact
	err := row.Scan(&c.AccountID, &c.RemoteID, &c.Name)

	return c, err
}
