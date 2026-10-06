// These tests use package store, not store_test, because a failing
// migration cannot be produced through the public API.
package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpen_RejectsNewerSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "future.db")
	db := openRaw(t, path)
	if _, err := db.Exec(`PRAGMA user_version=1000`); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(t.Context(), path); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open = %v, want ErrSchemaTooNew", err)
	}
}

func TestMigrate_FailedStepKeepsLastGoodVersion(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "messages.db"))
	steps := []string{`CREATE TABLE good(id INTEGER)`, `CREATE TABLE broken(`}

	if err := migrate(t.Context(), db, steps); err == nil {
		t.Fatal("migrate with a broken step succeeded")
	}

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("user_version = %d (%v), want 1", version, err)
	}

	if err := migrate(t.Context(), db, steps[:1]); err != nil {
		t.Fatalf("migrate after the failure = %v, want nil", err)
	}
}

// openRaw opens a SQLite database without migrating it.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}
