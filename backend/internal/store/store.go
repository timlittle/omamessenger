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

// Logger receives one diagnostic line when Open tightens the data
// directory's permissions. *log.Logger already satisfies it; a nil
// logger, as most tests pass, just skips that line.
type Logger interface {
	Printf(format string, v ...any)
}

// Open creates the database file and its directory, readable only by the
// owner, and applies any pending migrations. When the directory already
// exists looser than owner-only - a cold install's installer can leave
// it at the default mode before creating it private - Open tightens it
// and reports the change through logger.
func Open(ctx context.Context, path string, logger Logger) (*Store, error) {
	if err := createPrivateFile(path, logger); err != nil {
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

// createPrivateFile makes sure path's directory and the file itself
// exist with owner-only permissions before SQLite opens it, because
// SQLite would create the file world-readable.
func createPrivateFile(path string, logger Logger) error {
	if err := securePrivateDir(filepath.Dir(path), logger); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	return f.Close()
}

// securePrivateDir creates dir at 0700 when it does not exist yet, and
// tightens it to 0700 when it already exists readable or writable by
// the group or others, logging the change through logger - never the
// path itself, which stays out of logs. A cold install's installer can
// leave the data directory at the default mode before this function
// first runs against it; see scripts/install-helper.sh for the
// installer's own half of this. A directory missing one of the owner's
// own bits (such as a read-only test fixture) is left alone: that is a
// different problem, and Open fails on it in its own way below.
func securePrivateDir(dir string, logger Logger) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}

	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}

	if logger != nil {
		logger.Printf("store: tightened the data directory's permissions")
	}

	return os.Chmod(dir, 0o700) //nolint:gosec // deliberate: 0700 is a directory mode (owner rwx, no group or other access); gosec's G302 does not distinguish it from an overly open file mode
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
