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
	// Full-text search over message bodies. The table holds no data of its
	// own (content='messages'): it indexes the messages table in place, so
	// triggers must keep it in step, and a rebuild indexes what already
	// exists. remove_diacritics 2 folds accents and case; the prefix
	// indexes speed up the short prefixes a search box types first.
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
	// milliseconds), or 0 for none: local to this computer only, like
	// hidden.
	{sql: `ALTER TABLE conversations ADD COLUMN reminder_at INTEGER NOT NULL DEFAULT 0;`},
	// A message's @-mention tokens, as JSON, the same way reactions are
	// stored, and whether any of them names the signed-in account.
	{sql: `ALTER TABLE messages ADD COLUMN mentions TEXT NOT NULL DEFAULT '';
	ALTER TABLE messages ADD COLUMN mentions_me INTEGER NOT NULL DEFAULT 0;`},
	// Automatic retry for a failed send: when its next attempt is due
	// (0 for none scheduled), how many automatic attempts the current
	// failure streak has had, and when that streak began, so the
	// backoff and its bound survive a restart. The file an attachment
	// was attached from, with its size and modification time, so a
	// retry can re-copy it if the outgoing area's own copy goes
	// missing; empty for a clipboard paste, which has no such file.
	{sql: `ALTER TABLE messages ADD COLUMN retry_at INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN retry_attempts INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN retry_since INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE messages ADD COLUMN attachment_original_path TEXT NOT NULL DEFAULT '';
	ALTER TABLE messages ADD COLUMN attachment_original_modtime INTEGER NOT NULL DEFAULT 0;`},
	// The due time (Unix milliseconds) a reminder has already been
	// notified for, or 0 for none yet. A reminder itself is never
	// cleared automatically once it fires (see docs/decisions.md), so
	// this is the scheduler's own record of having already announced
	// it, kept in the database rather than in memory so a restart does
	// not announce the same still-due reminder a second time.
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
	// starting with the version the user last saw the in-app "What's
	// new" list for (see store.SeenWhatsNewVersion), so the one-time
	// update hint shows at most once per release.
	{sql: `CREATE TABLE ui_state(id INTEGER PRIMARY KEY CHECK (id = 1), seen_whats_new_version TEXT NOT NULL DEFAULT '');
	INSERT INTO ui_state(id) VALUES (1);`},
	// A document a connector classified as a plain file before it
	// learned to recognise a real GIF image by its mime type or name
	// (see telegram/media.go's and whatsapp/normalize_media.go's own
	// isGIFImage) is reclassified here the same way: MediaVideo,
	// Animated, so it reaches the same inline view and the same
	// sandboxed play copy on its next fetch as one received after
	// that fix, rather than staying a plain file chip forever. Only a
	// stored file name is available to judge this by - mime type was
	// never persisted - which is exactly the fallback isGIFImage
	// itself uses when a sender's mime type is missing. This never
	// decides how the UI plays anything: that still only ever comes
	// from the downloaded bytes, re-encoded fresh by the transcoder on
	// every fetch (see app.Commands.playCopy), so a wrongly
	// reclassified row can only ever fall back to opening externally,
	// never play unsafely.
	{sql: `UPDATE messages
		SET media = json_set(media, '$.kind', 'video', '$.animated', json('true'))
		WHERE json_valid(media)
			AND json_extract(media, '$.kind') = 'file'
			AND lower(json_extract(media, '$.fileName')) LIKE '%.gif';`},
	// A WhatsApp audio message that arrived before this connector told
	// an ordinary audio file apart from a voice note (see
	// whatsapp/normalize_media.go's audioMedia) is reclassified here:
	// MediaAudio, so it reaches the same inline player its push-to-talk
	// sibling already gets on its next fetch, rather than staying a
	// plain file chip forever. "audio-message.ogg" is the helper's own
	// fixed name for this case, never a sender's choice, so matching it
	// is safe the way matching a sender's own claimed file name would
	// not be - the sandboxed transcoder still decides whether the
	// reclassified row actually plays, exactly as it already does for
	// every "voice" or "audio" row. A Telegram audio document stored
	// this way carries only the sender's own file name, with nothing
	// that would safely tell it apart from an ordinary file, so it is
	// left as it is.
	{sql: `UPDATE messages
		SET media = json_set(media, '$.kind', 'audio')
		WHERE json_valid(media)
			AND json_extract(media, '$.kind') = 'file'
			AND json_extract(media, '$.fileName') = 'audio-message.ogg';`},
	// Where a pinned conversation sits among the others pinned, lowest
	// first; 0 for a conversation that is not pinned, which never reads
	// this column (see store.SetPinned and store.MovePin). Unlimited
	// pinning (issue #14) needs an explicit order because "newest
	// activity first", the rule that already orders unpinned chats,
	// gives a pinned chat no stable position of its own to drag or move.
	{sql: `ALTER TABLE conversations ADD COLUMN pin_order INTEGER NOT NULL DEFAULT 0;`},
	// Messages the user queued to send at a chosen time instead of right
	// away (see app.Commands.ScheduleMessage). Local to this computer
	// only, like reminder_at above: a row here is removed, not updated
	// to some "sent" state, once it is claimed and handed to the normal
	// send path (see store/scheduled.go and docs/decisions.md), so there
	// is no status column to add.
	{sql: `CREATE TABLE scheduled_messages(
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		text TEXT NOT NULL,
		send_at INTEGER NOT NULL,
		created INTEGER NOT NULL);
	CREATE INDEX scheduled_messages_due ON scheduled_messages(send_at);`},
	// "" (the default) inherits whichever notification sound applies at
	// the service or global default level; "custom" means this
	// conversation has its own, stored by the helper under its own id
	// (see cache.Sounds and app.Commands.SetNotificationSound) rather
	// than kept in this column, which only ever records that one has
	// been set, never a path (issue #19).
	{sql: `ALTER TABLE conversations ADD COLUMN notification_sound TEXT NOT NULL DEFAULT '';`},
	// Whether the window was last left open rather than deliberately
	// closed (see store.WindowOpen), so a shell restart or plugin reload
	// that destroys it without the user asking to close it can reopen it
	// on the next start.
	{sql: `ALTER TABLE ui_state ADD COLUMN window_open INTEGER NOT NULL DEFAULT 0;`},
	// Every conversation pinned before the pin_order column above
	// existed shares its bare default, 0: MovePin's own swap had
	// nothing to swap, since exchanging 0 for 0 leaves both at 0, so
	// the first real move a user with old pins made looked like it did
	// nothing at all. This gives each one a distinct order instead, by
	// the same recency order the list already showed them in, so the
	// migration itself reorders nothing visible; only a later move can.
	{sql: `UPDATE conversations SET pin_order = (
			SELECT rank FROM (
				SELECT id, ROW_NUMBER() OVER (ORDER BY last_activity DESC, id) AS rank
				FROM conversations WHERE pinned = 1
			) ranked
			WHERE ranked.id = conversations.id)
		WHERE pinned = 1;`},
	// A pin SetArchived cleared locally to match a service that
	// unpins on archive (see docs/decisions.md), remembered so
	// unarchiving can restore it: pinned_before_archive and
	// pin_order_before_archive hold what pinned and pin_order were
	// the moment this account's own service required that unpin,
	// and are both cleared again once restored (see
	// RememberPinnedBeforeArchive and RestorePinnedFromArchive).
	// Meaningless, and left at their default of 0, for a
	// conversation that was not pinned when it was archived.
	{sql: `ALTER TABLE conversations ADD COLUMN pinned_before_archive INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE conversations ADD COLUMN pin_order_before_archive INTEGER NOT NULL DEFAULT 0;`},
	// A thumbnail stored before imagesafe.Sanitize existed - or written
	// by any version that, for whatever reason, saved one unchecked -
	// re-sanitized the same way a connector already sanitizes one the
	// moment it receives it, or cleared when it fails that check
	// outright (see sanitizeLegacyThumbnails and docs/decisions.md).
	// scanMessage's own call to sanitizedThumb keeps making this
	// guarantee for every row from here on, so this step only ever
	// needs to run once.
	{repair: sanitizeLegacyThumbnails},
	// A reply quote stored before it kept its own copy of the quoted
	// message's sender id - see domain.Reply.SenderID and
	// backfillReplySenderIDs - resolved the same way resolveReply
	// resolves one for a brand new reply, within its own conversation
	// only. RefreshSenderName's own scoping depends on this: without
	// it, every quote stored before this field existed would stay
	// unmatchable by sender id forever, exactly as an unresolved reply
	// already is (see docs/decisions.md). resolveReply keeps making
	// this guarantee for every reply from here on, so this step only
	// ever needs to run once.
	{repair: backfillReplySenderIDs},
	// Whether a message's sender name, and a conversation's title, are
	// verified - a saved contact, a service's own verified business
	// name, or the user's own identity, rather than a bare, self-chosen
	// name a stranger could set to "Mum" or "You" (see
	// domain.Message.SenderVerified and docs/decisions.md). A row
	// stored before this column existed defaults to verified: the same
	// trust an older release already showed it with, not a retroactive
	// downgrade with no new information behind it. Only a name resolved
	// from here on is held to the stricter check. A reply quote's own
	// SenderVerified lives inside its reply_to JSON instead, the same
	// way its SenderName already does, so it needs no column of its
	// own here.
	{sql: `ALTER TABLE messages ADD COLUMN sender_verified INTEGER NOT NULL DEFAULT 1;
		ALTER TABLE conversations ADD COLUMN title_verified INTEGER NOT NULL DEFAULT 1;`},
	// A direct chat's other person's own phone number, formatted
	// "+<digits>", when the service gives one - never a group's or the
	// self-chat's, which have none; shown beside an unverified title
	// the way the official WhatsApp app shows a stranger's number
	// instead of trusting their self-chosen name.
	{sql: `ALTER TABLE conversations ADD COLUMN phone TEXT NOT NULL DEFAULT '';`},
	// A follow-up security review found the migration above wrong to
	// default an upgraded row to verified: that is the same trust
	// level as a name this connector has actually resolved and checked,
	// for a row this helper has never checked at all. A stranger who
	// messaged before the upgrade, naming themselves "Mum", kept
	// showing unmarked forever, since nothing about an upgrade itself
	// re-resolves their name. This step resets every existing row to
	// unverified instead, the secure default for a row of unknown
	// trust; a saved contact's messages and chat title are re-marked
	// verified the next time this connector resolves their name, which
	// WhatsApp's own AppStateSyncComplete event (see
	// whatsapp/contacts.go's handleAppStateSyncComplete) already
	// triggers for every known direct chat shortly after each
	// reconnect, with no need to wait for their next message.
	{sql: `UPDATE messages SET sender_verified=0;
		UPDATE conversations SET title_verified=0;`},
	// A security review found the new-chat picker showing every
	// contact unmarked, with no phone number, however trustworthy
	// its own name actually was: contacts never carried either column
	// at all, so UpsertContact had nothing to write and Contact kept
	// whatever its caller defaulted it to, regardless of what the
	// connector actually reported. Both new columns default to
	// unverified and no phone, the same secure-by-default choice as
	// the reset above, rather than DEFAULT 1: a contact synced before
	// this column existed is re-marked the next time its connector
	// reports it again, which for WhatsApp happens through the push
	// name sync history delivers from time to time (see
	// whatsapp/history.go's syncPushnames).
	{sql: `ALTER TABLE contacts ADD COLUMN verified INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE contacts ADD COLUMN phone TEXT NOT NULL DEFAULT '';`},
	// A security review found a group's conversation-list preview
	// showing its last sender's own self-chosen name with no "~" mark
	// at all, even though the same name shows marked inside the chat
	// itself: preview_sender carried no verified flag of its own to
	// show one with. Defaults to unverified, the same secure-by-default
	// choice as every other column added for this review, since a row
	// stored before this column existed was never actually checked
	// either.
	{sql: `ALTER TABLE conversations ADD COLUMN preview_sender_verified INTEGER NOT NULL DEFAULT 0;`},
	// sender_verified, title_verified, phone (on both conversations and
	// contacts), contacts.verified and preview_sender_verified, all
	// added above, are no longer read or written by any Go code: the
	// UI stopped reading them, and the plumbing that fed them was
	// later removed too (see docs/decisions.md). Migrations are never
	// edited or reversed, so these columns stay in the schema, simply
	// unused, rather than getting a migration of their own to drop
	// them.
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
