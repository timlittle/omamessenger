package store

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// contactColumns lists the columns scanContact reads, in order.
const contactColumns = `account_id,remote_id,name,verified,phone`

// UpsertContact records a contact, or renames an existing one. Verified
// and Phone are overwritten too, not just Name: a contact a connector
// reports again always carries its current trust and phone number, and
// a security review found the picker showing every contact unmarked
// traced back to this store never keeping either at all (see
// docs/decisions.md).
func (s *Store) UpsertContact(ctx context.Context, c domain.Contact) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO contacts(`+contactColumns+`) VALUES(?,?,?,?,?)
		ON CONFLICT(account_id,remote_id) DO UPDATE SET name=excluded.name,verified=excluded.verified,phone=excluded.phone`,
		c.AccountID, c.RemoteID, c.Name, boolInt(c.Verified), c.Phone)

	return wrap("upsert contact", err)
}

// Contact returns one contact of an account.
func (s *Store) Contact(ctx context.Context, accountID, remoteID string) (domain.Contact, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+contactColumns+` FROM contacts
		WHERE account_id=? AND remote_id=?`, accountID, remoteID)
	c, err := scanContact(row)

	return c, wrap("contact", err)
}

// Contacts returns an account's contacts whose name contains query,
// ignoring case, ordered by name, capped at domain.MaxContactResults:
// a service's own contact sync fully controls how many contacts an
// account ends up with, with no limit of its own.
func (s *Store) Contacts(ctx context.Context, accountID, query string) ([]domain.Contact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+contactColumns+` FROM contacts
		WHERE account_id=? AND name LIKE ? ESCAPE '\' ORDER BY name COLLATE NOCASE LIMIT ?`,
		accountID, likePattern(query), domain.MaxContactResults)
	if err != nil {
		return nil, wrap("contacts", err)
	}

	contacts, err := scanAll(rows, scanContact)

	return contacts, wrap("contacts", err)
}

// scanContact reads one row selected with contactColumns. Name is
// sender-controlled - a service's own contact sync, not something this
// account typed - so it gets the same domain.CapDisplayName cap on
// every read that app.Ingest.Contact already gives it on arrival,
// repairing a row an older release stored before that cap existed.
func scanContact(row scanner) (domain.Contact, error) {
	var c domain.Contact
	var verified bool
	if err := row.Scan(&c.AccountID, &c.RemoteID, &c.Name, &verified, &c.Phone); err != nil {
		return c, err
	}

	c.Name = domain.CapDisplayName(c.Name)
	c.Verified = verified

	return c, nil
}
