package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// migrations are applied in order, and PRAGMA user_version records how many
// have run. Never edit a released migration: append a new one.
var migrations = []string{
	`CREATE TABLE accounts(
		id TEXT PRIMARY KEY,
		service TEXT NOT NULL,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'offline',
		detail TEXT NOT NULL DEFAULT '');
	CREATE TABLE contacts(
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL,
		name TEXT NOT NULL,
		PRIMARY KEY(account_id, remote_id));
	CREATE TABLE conversations(
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		members INTEGER NOT NULL DEFAULT 0,
		preview TEXT NOT NULL DEFAULT '',
		preview_sender TEXT NOT NULL DEFAULT '',
		preview_out INTEGER NOT NULL DEFAULT 0,
		unread INTEGER NOT NULL DEFAULT 0,
		muted INTEGER NOT NULL DEFAULT 0,
		last_activity INTEGER NOT NULL DEFAULT 0,
		UNIQUE(account_id, remote_id));
	CREATE TABLE messages(
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL DEFAULT '',
		sender_id TEXT NOT NULL DEFAULT '',
		sender_name TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL,
		outgoing INTEGER NOT NULL,
		status TEXT NOT NULL,
		created INTEGER NOT NULL);
	CREATE UNIQUE INDEX messages_remote ON messages(conversation_id, remote_id) WHERE remote_id != '';
	CREATE INDEX messages_timeline ON messages(conversation_id, created);`,
	// A message's link preview, photo, video or file, as JSON.
	`ALTER TABLE messages ADD COLUMN media TEXT NOT NULL DEFAULT '';`,
	// Full-text search over message bodies. The table holds no data of its
	// own (content='messages'): it indexes the messages table in place, so
	// triggers must keep it in step, and a rebuild indexes what already
	// exists. remove_diacritics 2 folds accents and case; the prefix
	// indexes speed up the short prefixes a search box types first.
	`CREATE VIRTUAL TABLE messages_fts USING fts5(text,
			content='messages', content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2', prefix='2 3');
		CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END;
		CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
		END;
		CREATE TRIGGER messages_fts_update AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
			INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
		END;
		INSERT INTO messages_fts(messages_fts) VALUES ('rebuild');`,
	// Whether a message has been edited since it was first stored.
	`ALTER TABLE messages ADD COLUMN edited INTEGER NOT NULL DEFAULT 0;`,
}

// ErrSchemaTooNew reports a database written by a newer helper. Opening it
// would risk losing data, so the helper refuses.
var ErrSchemaTooNew = errors.New("database schema is newer than this helper supports")

// migrate brings db up to date with steps, one transaction per step, so a
// failed step leaves the schema at the last good version.
func migrate(ctx context.Context, db *sql.DB, steps []string) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version > len(steps) {
		return fmt.Errorf("%w: found %d, supported %d", ErrSchemaTooNew, version, len(steps))
	}

	for i := version; i < len(steps); i++ {
		if err := applyStep(ctx, db, i+1, steps[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}

	return nil
}

// applyStep runs one migration and records its version atomically.
func applyStep(ctx context.Context, db *sql.DB, version int, step string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, step); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version=%d`, version)); err != nil {
		return err
	}

	return tx.Commit()
}
