package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestOpen_CreatesPrivateFilesAndReopens(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "private", "messages.db")
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	account := domain.Account{ID: "local", Service: domain.ServiceWhatsApp, Name: "Local"}
	if err := s.UpsertAccount(t.Context(), account); err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)

	s, err = store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := s.Account(t.Context(), "local")
	if err != nil || got.Name != "Local" {
		t.Fatalf("reopened Account = %+v, %v", got, err)
	}
}

func TestOpen_RejectsUnusablePaths(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Open(t.Context(), filepath.Join(file, "messages.db")); err == nil {
		t.Error("Open under a regular file succeeded")
	}

	if os.Geteuid() == 0 {
		return // root ignores directory permissions
	}

	readOnly := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Open(t.Context(), filepath.Join(readOnly, "messages.db")); err == nil {
		t.Error("Open in a read-only directory succeeded")
	}
}

// TestStore_ReportsDatabaseErrors checks that a storage failure reaches the
// caller as an error, and is never swallowed or mistaken for not found.
func TestStore_ReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	for name, call := range storeCalls(t, s) {
		if err := call(); err == nil || errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s on a closed database = %v, want a storage error", name, err)
		}
	}
}

// storeCalls invokes every Store method with valid-looking arguments.
func storeCalls(t *testing.T, s *store.Store) map[string]func() error {
	ctx := t.Context()
	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}
	contact := domain.Contact{AccountID: "wa", RemoteID: "r", Name: "N"}
	conversation := domain.Conversation{AccountID: "wa", RemoteID: "r", Title: "Chat"}
	message := domain.Message{ConversationID: "c", Text: "hi"}
	remote := domain.Message{ConversationID: "c", RemoteID: "remote", Text: "hi"}

	return map[string]func() error{
		"UpsertAccount":        func() error { return s.UpsertAccount(ctx, account) },
		"DeleteAccount":        func() error { return s.DeleteAccount(ctx, "wa") },
		"SetAccountStatus":     func() error { _, err := s.SetAccountStatus(ctx, "wa", "connected", ""); return err },
		"Account":              func() error { _, err := s.Account(ctx, "wa"); return err },
		"Accounts":             func() error { _, err := s.Accounts(ctx); return err },
		"UpsertContact":        func() error { return s.UpsertContact(ctx, contact) },
		"Contact":              func() error { _, err := s.Contact(ctx, "wa", "r"); return err },
		"Contacts":             func() error { _, err := s.Contacts(ctx, "wa", ""); return err },
		"EnsureConversation":   func() error { _, _, err := s.EnsureConversation(ctx, conversation); return err },
		"Conversation":         func() error { _, err := s.Conversation(ctx, "c"); return err },
		"ConversationByRemote": func() error { _, err := s.ConversationByRemote(ctx, "wa", "r"); return err },
		"Conversations":        func() error { _, err := s.Conversations(ctx, "q"); return err },
		"MarkRead":             func() error { _, err := s.MarkRead(ctx, "c"); return err },
		"SetMuted":             func() error { return s.SetMuted(ctx, "c", true) },
		"SetPinned":            func() error { return s.SetPinned(ctx, "c", true) },
		"SetArchived":          func() error { return s.SetArchived(ctx, "c", true) },
		"UnreadTotal":          func() error { _, err := s.UnreadTotal(ctx); return err },
		"AddMessage":           func() error { _, _, err := s.AddMessage(ctx, message); return err },
		"AddMessage remote":    func() error { _, _, err := s.AddMessage(ctx, remote); return err },
		"Message":              func() error { _, err := s.Message(ctx, "m"); return err },
		"MessageByRemote":      func() error { _, err := s.MessageByRemote(ctx, "c", "remote"); return err },
		"SetMessageRemoteID":   func() error { return s.SetMessageRemoteID(ctx, "m", "remote") },
		"UpdateMessageStatus":  func() error { _, _, err := s.UpdateMessageStatus(ctx, "m", domain.StatusSent); return err },
		"Messages":             func() error { _, _, err := s.Messages(ctx, "c", "", 10); return err },
		"Messages before":      func() error { _, _, err := s.Messages(ctx, "c", "m", 10); return err },
	}
}

// assertMode fails the test unless path has exactly the given permissions.
func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := info.Mode().Perm(); got != want {
		t.Errorf("permissions of %s = %04o, want %04o", path, got, want)
	}
}
