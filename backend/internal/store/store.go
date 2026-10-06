// Package store persists the normalized domain in SQLite.
//
// Timestamps are integer Unix milliseconds. Ordering always breaks ties on
// rowid so two messages in the same millisecond keep their arrival order.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// Store is the SQLite database holding accounts, contacts, conversations
// and messages. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// Open creates the database file and its directory, readable only by the
// owner, and applies any pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := createPrivateFile(path); err != nil {
		return nil, fmt.Errorf("store: create %s: %w", path, err)
	}

	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	// SQLite allows one writer; a single connection avoids "database is
	// locked" errors between our own goroutines.
	db.SetMaxOpenConns(1)

	if err := migrate(ctx, db, migrations); err != nil {
		return nil, errors.Join(fmt.Errorf("store: %w", err), db.Close())
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// createPrivateFile makes sure path exists with owner-only permissions
// before SQLite opens it, because SQLite would create it world-readable.
func createPrivateFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	return f.Close()
}

// wrap turns sql.ErrNoRows into domain.ErrNotFound and wraps any other
// error with the operation name.
func wrap(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("store: %s: %w", op, domain.ErrNotFound)
	default:
		return fmt.Errorf("store: %s: %w", op, err)
	}
}

// rowsChanged reports whether a statement changed any rows.
func rowsChanged(op string, res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: %s: %w", op, err)
	}

	return n > 0, nil
}

// requireRow returns domain.ErrNotFound when a statement changed no rows.
func requireRow(op string, res sql.Result) error {
	changed, err := rowsChanged(op, res)
	if err != nil || changed {
		return err
	}

	return fmt.Errorf("store: %s: %w", op, domain.ErrNotFound)
}

// newID returns a random identifier with a readable prefix, such as
// "m_4ZQ3…" for a message.
func newID(prefix string) string {
	return prefix + "_" + rand.Text()
}

// boolInt converts a bool to SQLite's integer form.
func boolInt(v bool) int {
	if v {
		return 1
	}

	return 0
}

// likePattern turns user input into a LIKE pattern that matches it anywhere,
// treating '%' and '_' literally. Queries must say ESCAPE '\'.
func likePattern(query string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(query))

	return "%" + escaped + "%"
}
