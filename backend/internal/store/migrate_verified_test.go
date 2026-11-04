// This test uses package store, not store_test, for the same reason
// migrate_test.go does: a failing or partial migration cannot be
// produced through the public API.
package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrate_DefaultsSenderAndTitleVerifiedToTrueForExistingRows
// confirms the verified columns' own column default: a message or
// conversation stored before sender_verified and title_verified
// existed reads back trusted immediately after those columns are
// added, matching the DEFAULT 1 they were given. A later security
// review found trusting that default forever was wrong - a stranger
// who messaged before the upgrade, naming themselves "Mum", stayed
// unmarked with no new information behind that trust - so the very
// next migration resets it back to unverified once all migrations have
// run (see the reset step at the end of migrations, and
// docs/decisions.md); this test only checks the intermediate state the
// reset itself corrects.
func TestMigrate_DefaultsSenderAndTitleVerifiedToTrueForExistingRows(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "verified-default.db"))
	// Stop four short of the full set: the verified columns are
	// fourth-from-last, appended after the reply sender id backfill and
	// before the conversation phone column, the verified-by-default
	// reset and the contact verified and phone columns.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-4]); err != nil {
		t.Fatal(err)
	}

	insertRawAccount(t, db, "wa")
	insertRawConversation(t, db, rawConversation{id: "c1", accountID: "wa", lastActivity: 1000})
	insertRawMessage(t, db, "m1", "")

	// Apply only the verified columns' own migration, not the phone
	// column or the reset after it, to see the column default alone.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-3]); err != nil {
		t.Fatal(err)
	}

	if got := readRawVerified(t, db, "m1", "c1"); got != [2]bool{true, true} {
		t.Errorf("verified flags right after the column default = %v, want both true", got)
	}

	// Running every migration, including the reset appended after this
	// one, must leave nothing still trusted by that old default alone.
	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	if got := readRawVerified(t, db, "m1", "c1"); got != [2]bool{false, false} {
		t.Errorf("verified flags once every migration has run = %v, want both false", got)
	}
}

// TestMigrate_ResetsVerifiedByDefaultRowsToUnverified confirms the
// migration added after the sender/title verified columns and the
// phone column resets every existing row to unverified, undoing the
// DEFAULT 1 those columns were given when first added (see
// docs/decisions.md): a security review found that default left a
// stranger's pre-upgrade "Mum" message trusted forever, the same as a
// name this connector had actually checked.
func TestMigrate_ResetsVerifiedByDefaultRowsToUnverified(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "verified-reset.db"))
	// Stop two short of the full set: the reset is second-from-last,
	// appended after the sender/title verified columns and the
	// conversation phone column, both already present by this point,
	// and before the contact verified and phone columns.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-2]); err != nil {
		t.Fatal(err)
	}

	insertRawAccount(t, db, "wa")
	// insertRawMessage always addresses conversation_id "c1", so the
	// conversation under test keeps that id too.
	insertRawConversation(t, db, rawConversation{id: "c1", accountID: "wa", lastActivity: 1})
	insertRawMessage(t, db, "stranger-message", "")

	// Both columns default to verified before this migration runs,
	// exactly as an older release left them.
	if got := readRawVerified(t, db, "stranger-message", "c1"); got != [2]bool{true, true} {
		t.Fatalf("pre-reset verified flags = %v, want both true, matching the column default", got)
	}

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	if got := readRawVerified(t, db, "stranger-message", "c1"); got != [2]bool{false, false} {
		t.Errorf("post-reset verified flags = %v, want both false", got)
	}
}

// readRawVerified reads a message's sender_verified and its
// conversation's title_verified directly, for the reset migration test.
func readRawVerified(t *testing.T, db *sql.DB, messageID, conversationID string) [2]bool {
	t.Helper()

	var senderVerified, titleVerified bool
	row := db.QueryRowContext(t.Context(), `SELECT
		(SELECT sender_verified FROM messages WHERE id=?),
		(SELECT title_verified FROM conversations WHERE id=?)`, messageID, conversationID)
	if err := row.Scan(&senderVerified, &titleVerified); err != nil {
		t.Fatal(err)
	}

	return [2]bool{senderVerified, titleVerified}
}
