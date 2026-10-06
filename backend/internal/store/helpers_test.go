package store_test

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// openStore opens a fresh database in a temporary directory and closes it
// when the test ends.
func openStore(t *testing.T) *store.Store {
	t.Helper()

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "data", "messages.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	return s
}

// addAccount stores a WhatsApp account named after its id.
func addAccount(t *testing.T, s *store.Store, id string) {
	t.Helper()

	account := domain.Account{ID: id, Service: domain.ServiceWhatsApp, Name: id}
	if err := s.UpsertAccount(t.Context(), account); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
}

// addConversation stores a direct conversation whose remote id is "r-" + id.
func addConversation(t *testing.T, s *store.Store, accountID, id, title string) domain.Conversation {
	t.Helper()

	c, created, err := s.EnsureConversation(t.Context(), domain.Conversation{
		ID: id, AccountID: accountID, RemoteID: "r-" + id, Title: title, Kind: domain.KindDirect,
	})
	if err != nil || !created {
		t.Fatalf("EnsureConversation(%q) = created %t, %v", id, created, err)
	}

	return c
}

// addMessages stores each message, failing the test on any error.
func addMessages(t *testing.T, s *store.Store, messages ...domain.Message) {
	t.Helper()

	for _, m := range messages {
		if _, _, err := s.AddMessage(t.Context(), m); err != nil {
			t.Fatalf("AddMessage(%q): %v", m.ID, err)
		}
	}
}

// ids returns the id of each message, in order.
func ids(messages []domain.Message) []string {
	out := make([]string, len(messages))
	for i, m := range messages {
		out[i] = m.ID
	}

	return out
}
