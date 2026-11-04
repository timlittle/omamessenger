package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// migrationStep is one change migrate applies and records in PRAGMA
// user_version: sql for a plain schema or data change text alone can
// express, or repair for one that needs Go code to decide what a row
// should become, such as sanitizeLegacyThumbnails. Exactly one of the
// two is set.
type migrationStep struct {
	sql    string
	repair func(ctx context.Context, tx *sql.Tx) error
}

// migrations are applied in order, and PRAGMA user_version records how many
// have run. Never edit a released migration: append a new one.
var migrations = []migrationStep{
	{sql: `CREATE TABLE accounts(
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
	CREATE INDEX messages_timeline ON messages(conversation_id, created);`},
	// A message's link preview, photo, video or file, as JSON.
	{sql: `ALTER TABLE messages ADD COLUMN media TEXT NOT NULL DEFAULT '';`},
	// Full-text search over message bodies, indexing the messages table in
	// place; triggers keep it in step on insert, delete and update.
	{sql: `CREATE VIRTUAL TABLE messages_fts USING fts5(text,
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
		INSERT INTO messages_fts(messages_fts) VALUES ('rebuild');`},
	// Whether a message has been edited since it was first stored.
	{sql: `ALTER TABLE messages ADD COLUMN edited INTEGER NOT NULL DEFAULT 0;`},
	// A conversation pinned to the top of the list, or filed away in the
	// service's archive, as Telegram's dialogs report it.
	{sql: `ALTER TABLE conversations ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE conversations ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;`},
	// The message a reply answers: its remote id, sender and a short
	// excerpt, as JSON, the same way media is stored.
	{sql: `ALTER TABLE messages ADD COLUMN reply_to TEXT NOT NULL DEFAULT '';`},
	// A message's emoji reaction chips, as JSON.
	{sql: `ALTER TABLE messages ADD COLUMN reactions TEXT NOT NULL DEFAULT '';`},
	// A conversation hidden from the standard list by the user, local to
	// this computer: never reported to or read from the service.
	{sql: `ALTER TABLE conversations ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;`},
	// A conversation snoozed by the user until a chosen time (Unix
	// milliseconds), or 0 for none; local to this computer only, like hidden.
	{sql: `ALTER TABLE conversations ADD COLUMN reminder_at INTEGER NOT NULL DEFAULT 0;`},
	// A message's @-mention tokens, as JSON, the same way reactions are
	// stored, and whether any of them names the signed-in account.
	{sql: `ALTER TABLE messages ADD COLUMN mentions TEXT NOT NULL DEFAULT '';
	ALTER TABLE messages ADD COLUMN mentions_me INTEGER NOT NULL DEFAULT 0;`},
	// Automatic retry for a failed send (next attempt due, attempt count,
	// streak start) so backoff survives a restart, plus the original
	// attachment file and modtime so a retry can re-copy it.
	{sql: `ALTER TABLE messages ADD COLUMN retry_at INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN retry_attempts INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN retry_since INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN attachment_original_path TEXT NOT NULL DEFAULT '';
	ALTER TABLE messages ADD COLUMN attachment_original_modtime INTEGER NOT NULL DEFAULT 0;`},
	// The due time (Unix milliseconds) a reminder has already been
	// notified for, or 0 for none yet, so a restart does not announce
	// the same still-due reminder twice.
	{sql: `ALTER TABLE conversations ADD COLUMN reminder_notified_at INTEGER NOT NULL DEFAULT 0;`},
	// A message's rich-text formatting spans (links, bold, italic,
	// underline, strike, code, pre, spoiler), as JSON, the same way
	// mentions are stored.
	{sql: `ALTER TABLE messages ADD COLUMN formatting TEXT NOT NULL DEFAULT '';`},
	// The display name the user chose for an account, such as
	// "Personal WhatsApp", shown instead of its plain name wherever it
	// is identified; empty until they set one.
	{sql: `ALTER TABLE accounts ADD COLUMN alias TEXT NOT NULL DEFAULT '';`},
	// Whether the account has blocked the contact behind a direct
	// conversation (see app.Commands.BlockContact and ReportSpam).
	{sql: `ALTER TABLE conversations ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0;`},
	// A single row of durable UI state that outlives a plugin reload,
	// starting with the version the user last saw the "What's new" list for.
	{sql: `CREATE TABLE ui_state(id INTEGER PRIMARY KEY CHECK (id = 1), seen_whats_new_version TEXT NOT NULL DEFAULT '');
	INSERT INTO ui_state(id) VALUES (1);`},
	// Reclassifies a GIF stored as a plain file (before isGIFImage existed)
	// as an animated video, by its file name, so it gets the inline view.
	// Playback itself still always goes through the sandboxed transcoder.
	{sql: `UPDATE messages
		SET media = json_set(media, '$.kind', 'video', '$.animated', json('true'))
		WHERE json_valid(media)
			AND json_extract(media, '$.kind') = 'file'
			AND lower(json_extract(media, '$.fileName')) LIKE '%.gif';`},
	// Reclassifies a WhatsApp voice note stored as a plain file (before
	// audioMedia told it apart) as audio, matching the helper's own fixed
	// file name "audio-message.ogg", never a sender-chosen one.
	{sql: `UPDATE messages
		SET media = json_set(media, '$.kind', 'audio')
		WHERE json_valid(media)
			AND json_extract(media, '$.kind') = 'file'
			AND json_extract(media, '$.fileName') = 'audio-message.ogg';`},
	// Where a pinned conversation sits among the others pinned, lowest
	// first; 0 for one not pinned. Needed because unpinned chats order by
	// recency alone, which gives a pinned chat nothing to drag or move.
	{sql: `ALTER TABLE conversations ADD COLUMN pin_order INTEGER NOT NULL DEFAULT 0;`},
	// Messages the user queued to send at a chosen time instead of right
	// away. A row is removed, not marked "sent", once it is claimed and
	// handed to the normal send path.
	{sql: `CREATE TABLE scheduled_messages(
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		text TEXT NOT NULL,
		send_at INTEGER NOT NULL,
		created INTEGER NOT NULL);
	CREATE INDEX scheduled_messages_due ON scheduled_messages(send_at);`},
	// "" inherits the service or global default sound; "custom" means
	// this conversation has its own, stored elsewhere by id, never a
	// path, in this column.
	{sql: `ALTER TABLE conversations ADD COLUMN notification_sound TEXT NOT NULL DEFAULT '';`},
	// Whether the window was last left open rather than deliberately
	// closed, so a shell restart or plugin reload can reopen it.
	{sql: `ALTER TABLE ui_state ADD COLUMN window_open INTEGER NOT NULL DEFAULT 0;`},
	// Every conversation pinned before pin_order existed shares its
	// default of 0, so MovePin's swap had nothing to swap. This gives
	// each a distinct order instead, matching the recency order already shown.
	{sql: `UPDATE conversations SET pin_order = (
			SELECT rank FROM (
				SELECT id, ROW_NUMBER() OVER (ORDER BY last_activity DESC, id) AS rank
				FROM conversations WHERE pinned = 1
			) ranked
			WHERE ranked.id = conversations.id)
		WHERE pinned = 1;`},
	// A pin SetArchived cleared locally (services unpin on archive, see
	// docs/decisions.md), remembered here so unarchiving can restore it.
	{sql: `ALTER TABLE conversations ADD COLUMN pinned_before_archive INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE conversations ADD COLUMN pin_order_before_archive INTEGER NOT NULL DEFAULT 0;`},
	// Re-sanitizes a thumbnail stored before imagesafe.Sanitize existed,
	// or clears it when it fails that check (see sanitizeLegacyThumbnails).
	{repair: sanitizeLegacyThumbnails},
	// Backfills a reply quote's sender id for one stored before it kept
	// its own copy (see domain.Reply.SenderID), the same way resolveReply
	// resolves a new reply, within its own conversation only.
	{repair: backfillReplySenderIDs},
	// Whether a message's sender name and a conversation's title are
	// verified, rather than a bare self-chosen name (see
	// domain.Message.SenderVerified). A row stored before this column
	// existed defaults to verified, matching its existing trust level.
	{sql: `ALTER TABLE messages ADD COLUMN sender_verified INTEGER NOT NULL DEFAULT 1;
		ALTER TABLE conversations ADD COLUMN title_verified INTEGER NOT NULL DEFAULT 1;`},
	// A direct chat's other person's phone number, formatted "+<digits>",
	// shown beside an unverified title. Never set for a group or self-chat.
	{sql: `ALTER TABLE conversations ADD COLUMN phone TEXT NOT NULL DEFAULT '';`},
	// A follow-up review found defaulting an upgraded row to verified
	// wrong: that is unearned trust for a row never actually checked.
	// Resets every existing row to unverified; a saved contact is
	// re-marked verified the next time its name is resolved.
	{sql: `UPDATE messages SET sender_verified=0;
		UPDATE conversations SET title_verified=0;`},
	// Adds verified and phone to contacts, which previously carried
	// neither; a contact synced before this column existed is re-marked
	// the next time its connector reports it again.
	{sql: `ALTER TABLE contacts ADD COLUMN verified INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE contacts ADD COLUMN phone TEXT NOT NULL DEFAULT '';`},
	// A group's conversation-list preview showed its sender's name with
	// no verified mark, since preview_sender carried no such flag.
	// Defaults to unverified, like the other columns added for this review.
	{sql: `ALTER TABLE conversations ADD COLUMN preview_sender_verified INTEGER NOT NULL DEFAULT 0;`},
	// sender_verified, title_verified, phone, contacts.verified and
	// preview_sender_verified are no longer read or written by any Go
	// code (see docs/decisions.md). Migrations are never reversed, so
	// they stay in the schema, unused.
}

// ErrSchemaTooNew reports a database written by a newer helper. Opening it
// would risk losing data, so the helper refuses.
var ErrSchemaTooNew = errors.New("database schema is newer than this helper supports")

// migrate brings db up to date with steps, one transaction per step, so a
// failed step leaves the schema at the last good version.
func migrate(ctx context.Context, db *sql.DB, steps []migrationStep) error {
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
func applyStep(ctx context.Context, db *sql.DB, version int, step migrationStep) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if err := runStep(ctx, tx, step); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version=%d`, version)); err != nil {
		return err
	}

	return tx.Commit()
}

// runStep executes step's SQL text, or its Go repair function when it
// has none of its own.
func runStep(ctx context.Context, tx *sql.Tx, step migrationStep) error {
	if step.repair != nil {
		return step.repair(ctx, tx)
	}

	_, err := tx.ExecContext(ctx, step.sql)

	return err
}
