// This test uses package store, not store_test, because a row that
// fails to scan cannot be produced through the public API: Store
// always writes valid JSON into the columns scanMessage decodes, so
// the only way to see a corrupt one is to write it directly with SQL.
package store

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestScanAll_ReportsARowThatFailsToScan confirms a stored message
// whose JSON column was corrupted outside this store - by another
// process, or a bug in an earlier version - surfaces as an error from
// Messages, rather than panicking or silently dropping the row.
func TestScanAll_ReportsARowThatFailsToScan(t *testing.T) {
	t.Parallel()

	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "wa"}
	if err := s.UpsertAccount(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	conv, _, err := s.EnsureConversation(t.Context(), domain.Conversation{ID: "c", AccountID: "wa", RemoteID: "r", Title: "Chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddMessage(t.Context(), domain.Message{ID: "m1", ConversationID: conv.ID, Text: "hi", Created: 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.ExecContext(t.Context(), `UPDATE messages SET mentions=? WHERE id=?`, "{not json", "m1"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.Messages(t.Context(), conv.ID, "", 10); err == nil {
		t.Error("Messages with a corrupted row = nil error, want the scan failure reported")
	}
}
