// These tests use package store, not store_test, because inspecting the
// query plan and timing a search are not reachable through the public
// API: they look at how Conversations' SQL executes, not just what it
// returns.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConversations_SearchUsesTheFTSIndex checks that searching messages
// goes through the messages_fts virtual table rather than scanning every
// row of the messages table: EXPLAIN QUERY PLAN must name messages_fts,
// not a plain SCAN of messages.
func TestConversations_SearchUsesTheFTSIndex(t *testing.T) {
	t.Parallel()

	s := seedSearchStore(t, 2000)

	rows, err := s.db.QueryContext(t.Context(), `EXPLAIN QUERY PLAN `+conversationsByTitleOrMessage,
		sql.Named("fts", `"ticket"*`), sql.Named("pattern", "%nomatch%"))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	got := plan.String()
	if !strings.Contains(got, "SCAN messages_fts VIRTUAL TABLE INDEX") {
		t.Errorf("query plan = %s, want a step scanning the messages_fts virtual table", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "SCAN messages ") || line == "SCAN messages" {
			t.Errorf("query plan = %s, want no full scan of the messages table", got)
		}
	}
}

// TestConversations_SearchScalesToThousandsOfMessages checks that a search
// over a few thousand messages still finds the match. The query plan test
// above is what actually guards performance here: this test only confirms
// correctness does not break down at that size.
func TestConversations_SearchScalesToThousandsOfMessages(t *testing.T) {
	t.Parallel()

	s := seedSearchStore(t, 5000)

	got, err := s.Conversations(t.Context(), "ticket")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Error("Conversations(ticket) found no match, want the seeded messages to match")
	}
}

// seedSearchStore opens a store with one conversation holding n messages,
// each containing the word "ticket", for the search performance tests.
func seedSearchStore(t *testing.T, n int) *Store {
	t.Helper()

	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "data", "messages.db"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "wa"}
	if err := s.UpsertAccount(t.Context(), account); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	conversation, _, err := s.EnsureConversation(t.Context(), domain.Conversation{
		ID: "chat", AccountID: "wa", RemoteID: "remote", Title: "Chat",
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	for i := range n {
		m := domain.Message{ConversationID: conversation.ID, Text: fmt.Sprintf("ticket number %d resolved", i), Created: int64(i)}
		if _, _, err := s.AddMessage(t.Context(), m); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}

	return s
}
