package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestOperationsReportDatabaseErrors checks that a storage failure reaches
// the caller as an error, and is never swallowed or mistaken for not-found.
func TestOperationsReportDatabaseErrors(t *testing.T) {
	s, _ := openTestStore(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}
	conversation := domain.Conversation{AccountID: "wa", RemoteID: "r", Title: "Chat"}
	message := domain.Message{ConversationID: "c", Text: "hi"}
	remoteMessage := domain.Message{ConversationID: "c", RemoteID: "remote", Text: "hi"}
	operations := map[string]func() error{
		"UpsertAccount":    func() error { return s.UpsertAccount(account) },
		"SetAccountStatus": func() error { _, err := s.SetAccountStatus("wa", "connected", ""); return err },
		"Account":          func() error { _, err := s.Account("wa"); return err },
		"Accounts":         func() error { _, err := s.Accounts(); return err },
		"UpsertContact": func() error {
			return s.UpsertContact(domain.Contact{AccountID: "wa", RemoteID: "r", Name: "N"})
		},
		"Contact":              func() error { _, err := s.Contact("wa", "r"); return err },
		"Contacts":             func() error { _, err := s.Contacts("wa", ""); return err },
		"EnsureConversation":   func() error { _, _, err := s.EnsureConversation(conversation); return err },
		"Conversation":         func() error { _, err := s.Conversation("c"); return err },
		"ConversationByRemote": func() error { _, err := s.ConversationByRemote("wa", "r"); return err },
		"Conversations":        func() error { _, err := s.Conversations("q"); return err },
		"MarkRead":             func() error { _, err := s.MarkRead("c"); return err },
		"SetMuted":             func() error { return s.SetMuted("c", true) },
		"UnreadTotal":          func() error { _, err := s.UnreadTotal(); return err },
		"AddMessage":           func() error { _, _, err := s.AddMessage(message); return err },
		"AddMessage remote":    func() error { _, _, err := s.AddMessage(remoteMessage); return err },
		"Message":              func() error { _, err := s.Message("m"); return err },
		"MessageByRemote":      func() error { _, err := s.MessageByRemote("c", "remote"); return err },
		"SetMessageRemoteID":   func() error { return s.SetMessageRemoteID("m", "remote") },
		"UpdateMessageStatus":  func() error { _, _, err := s.UpdateMessageStatus("m", domain.StatusSent); return err },
		"Messages":             func() error { _, _, err := s.Messages("c", "", 10); return err },
		"Messages before":      func() error { _, _, err := s.Messages("c", "m", 10); return err },
	}
	for name, operation := range operations {
		err := operation()
		if err == nil || errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s on a closed database = %v, want a storage error", name, err)
		}
	}
}

// TestFailedMigrationRollsBack checks a migration that fails leaves the
// schema at the last good version, so a fixed helper can migrate later.
func TestFailedMigrationRollsBack(t *testing.T) {
	s, path := openTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	saved := migrations
	t.Cleanup(func() { migrations = saved })
	migrations = append(append([]string{}, saved...), "CREATE TABLE broken(")
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "migration 2") {
		t.Fatalf("Open with a broken migration = %v, want a migration 2 error", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(saved) {
		t.Fatalf("user_version after failed migration = %d (%v), want %d", version, err, len(saved))
	}
}

func TestOpenReportsUnusablePaths(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(blocker, "messages.db")); err == nil {
		t.Error("Open under a regular file succeeded")
	}
	readOnly := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if _, err := Open(filepath.Join(readOnly, "messages.db")); err == nil {
			t.Error("Open in a read-only directory succeeded")
		}
	}
}
