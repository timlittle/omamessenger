package store

import (
	"errors"
	"fmt"
)

// migrations are applied in order; PRAGMA user_version records progress.
// Never edit a released migration: append a new one.
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
}

// schemaVersion is the version a freshly opened store reports.
var schemaVersion = len(migrations)

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema %d is newer than this helper supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, errors.Join(err, tx.Rollback()))
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, i+1)); err != nil {
			return errors.Join(err, tx.Rollback())
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
