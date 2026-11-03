// This test uses package store, not store_test, for the same reason
// migrate_test.go does: a failing or partial migration cannot be
// produced through the public API.
package store

import (
	"path/filepath"
	"testing"
)

// TestMigrate_DefaultsSenderAndTitleVerifiedToTrueForExistingRows
// confirms a message and a conversation stored before sender_verified
// and title_verified existed come back trusted, the same as an older
// release already showed them, rather than retroactively marked
// unverified with no new information behind that downgrade (see
// docs/decisions.md). Only a name resolved from here on is held to the
// stricter check.
func TestMigrate_DefaultsSenderAndTitleVerifiedToTrueForExistingRows(t *testing.T) {
	t.Parallel()

	db := openRaw(t, filepath.Join(t.TempDir(), "verified-default.db"))
	// Stop two short of the full set: the verified columns are
	// second-from-last, appended after the reply sender id backfill
	// and before the conversation phone column.
	if err := migrate(t.Context(), db, migrations[:len(migrations)-2]); err != nil {
		t.Fatal(err)
	}

	insertRawAccount(t, db, "wa")
	insertRawConversation(t, db, rawConversation{id: "c1", accountID: "wa", lastActivity: 1000})
	insertRawMessage(t, db, "m1", "")

	if err := migrate(t.Context(), db, migrations); err != nil {
		t.Fatal(err)
	}

	var senderVerified, titleVerified bool
	if err := db.QueryRowContext(t.Context(), `SELECT sender_verified FROM messages WHERE id='m1'`).Scan(&senderVerified); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT title_verified FROM conversations WHERE id='c1'`).Scan(&titleVerified); err != nil {
		t.Fatal(err)
	}

	if !senderVerified {
		t.Error("a message stored before sender_verified existed = false, want true")
	}
	if !titleVerified {
		t.Error("a conversation stored before title_verified existed = false, want true")
	}
}
