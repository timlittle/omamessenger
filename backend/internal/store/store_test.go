package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "messages.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close(): %v", err)
		}
	})
	return s, path
}

func seedAccount(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.UpsertAccount(domain.Account{ID: id, Service: domain.ServiceWhatsApp, Name: id}); err != nil {
		t.Fatalf("UpsertAccount(): %v", err)
	}
}

func seedConversation(t *testing.T, s *Store, accountID, id, remoteID, title string) domain.Conversation {
	t.Helper()
	c, created, err := s.EnsureConversation(domain.Conversation{
		ID: id, AccountID: accountID, RemoteID: remoteID, Title: title, Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(): %v", err)
	}
	if !created {
		t.Fatalf("EnsureConversation() created = false for %q", id)
	}
	return c
}

func TestOpenPermissionsMigrationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "messages.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAccount(domain.Account{ID: "local", Service: domain.ServiceWhatsApp, Name: "Local"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(path), 0o700}, {path, 0o600}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatalf("Stat(%q): %v", item.path, err)
		}
		if got := info.Mode().Perm(); got != item.mode {
			t.Errorf("permissions for %q = %04o, want %04o", item.path, got, item.mode)
		}
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.Account("local")
	if err != nil || a.Name != "Local" {
		t.Fatalf("reopened account = %#v, %v", a, err)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("user_version = %d, want %d", version, schemaVersion)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer than this helper supports") {
		t.Fatalf("Open() error = %v, want unsupported newer schema", err)
	}
}

func TestAccountsUpsertStatusAndValidation(t *testing.T) {
	s, _ := openTestStore(t)
	tests := []struct {
		name string
		acct domain.Account
	}{
		{name: "missing id", acct: domain.Account{Service: domain.ServiceWhatsApp, Name: "Name"}},
		{name: "missing name", acct: domain.Account{ID: "id", Service: domain.ServiceWhatsApp}},
		{name: "unsupported service", acct: domain.Account{ID: "id", Service: "signal", Name: "Name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.UpsertAccount(tt.acct); err == nil {
				t.Fatal("UpsertAccount() accepted invalid account")
			}
		})
	}

	if err := s.UpsertAccount(domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}); err != nil {
		t.Fatal(err)
	}
	a, err := s.Account("wa")
	if err != nil || a.Status != domain.AccountOffline {
		t.Fatalf("account default status = %#v, %v", a, err)
	}
	if err := s.UpsertAccount(domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Renamed", Status: domain.AccountConnected}); err != nil {
		t.Fatal(err)
	}
	a, err = s.Account("wa")
	if err != nil || a.Name != "Renamed" || a.Status != domain.AccountOffline {
		t.Fatalf("upserted account = %#v, %v", a, err)
	}
	a, err = s.SetAccountStatus("wa", domain.AccountConnected, "Ready")
	if err != nil || a.Status != domain.AccountConnected || a.Detail != "Ready" {
		t.Fatalf("SetAccountStatus() = %#v, %v", a, err)
	}
	if _, err := s.SetAccountStatus("missing", domain.AccountError, "broken"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetAccountStatus(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := s.Account("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Account(unknown) error = %v, want ErrNotFound", err)
	}
	accounts, err := s.Accounts()
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa" {
		t.Fatalf("Accounts() = %#v, %v", accounts, err)
	}
}

func TestEnsureConversationCreatesUpdatesAndValidates(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")

	input := domain.Conversation{ID: "chat", AccountID: "wa", RemoteID: "remote-chat", Title: "Old", Members: 2}
	created, isNew, err := s.EnsureConversation(input)
	if err != nil || !isNew {
		t.Fatalf("first EnsureConversation() = %#v, %t, %v", created, isNew, err)
	}
	if created.Kind != domain.KindDirect || created.Service != domain.ServiceWhatsApp {
		t.Errorf("created conversation defaults = %#v", created)
	}
	updated, isNew, err := s.EnsureConversation(domain.Conversation{
		AccountID: "wa", RemoteID: "remote-chat", Title: "New", Kind: domain.KindGroup, Members: 12,
	})
	if err != nil || isNew || updated.ID != "chat" || updated.Title != "New" || updated.Kind != domain.KindGroup || updated.Members != 12 {
		t.Fatalf("second EnsureConversation() = %#v, %t, %v", updated, isNew, err)
	}
	for _, invalid := range []domain.Conversation{
		{RemoteID: "remote", Title: "Title"},
		{AccountID: "wa", Title: "Title"},
		{AccountID: "wa", RemoteID: "remote"},
	} {
		if _, _, err := s.EnsureConversation(invalid); err == nil {
			t.Errorf("EnsureConversation(%#v) accepted missing fields", invalid)
		}
	}
	if _, err := s.Conversation("unknown"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Conversation(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := s.Conversation("unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Conversation(unknown) error = %v, want domain.ErrNotFound", err)
	}
	if _, err := s.ConversationByRemote("wa", "unknown"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ConversationByRemote(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestAddMessagePreviewUnreadDedupeAndCascade(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	seedConversation(t, s, "wa", "chat", "remote-chat", "Chat")

	add := func(m domain.Message) (domain.Message, bool) {
		t.Helper()
		got, inserted, err := s.AddMessage(m)
		if err != nil {
			t.Fatalf("AddMessage(%#v): %v", m, err)
		}
		return got, inserted
	}

	incoming, inserted := add(domain.Message{ID: "incoming", ConversationID: "chat", RemoteID: "r1", Text: "hello", SenderName: "Alex", Created: 200})
	if !inserted || incoming.Status != domain.StatusReceived {
		t.Fatalf("incoming = %#v, inserted=%t", incoming, inserted)
	}
	outgoing, inserted := add(domain.Message{ID: "outgoing", ConversationID: "chat", RemoteID: "r2", Text: "reply", Outgoing: true, Created: 300})
	if !inserted || outgoing.Status != domain.StatusPending {
		t.Fatalf("outgoing = %#v, inserted=%t", outgoing, inserted)
	}
	add(domain.Message{ID: "backfill", ConversationID: "chat", RemoteID: "r0", Text: "older history", SenderName: "Earlier", Created: 100})

	chat, err := s.Conversation("chat")
	if err != nil {
		t.Fatal(err)
	}
	if chat.Unread != 2 || chat.Preview != "reply" || !chat.PreviewOut || chat.PreviewSender != "" || chat.LastActivity != 300 {
		t.Errorf("conversation after messages = %#v", chat)
	}
	duplicate, inserted, err := s.AddMessage(domain.Message{ConversationID: "chat", RemoteID: "r1", Text: "duplicate", Created: 400})
	if err != nil || inserted || duplicate.ID != "incoming" {
		t.Fatalf("duplicate AddMessage() = %#v, %t, %v", duplicate, inserted, err)
	}
	chat, _ = s.Conversation("chat")
	if chat.Unread != 2 || chat.Preview != "reply" {
		t.Errorf("duplicate changed conversation: %#v", chat)
	}
	if _, _, err := s.AddMessage(domain.Message{ConversationID: "", Text: "text"}); err == nil {
		t.Error("AddMessage accepted a missing conversation id")
	}
	if _, _, err := s.AddMessage(domain.Message{ConversationID: "chat", Text: ""}); err == nil {
		t.Error("AddMessage accepted empty text")
	}
	if _, _, err := s.AddMessage(domain.Message{ConversationID: "missing", Text: "text"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddMessage(unknown conversation) error = %v, want ErrNotFound", err)
	}

	if _, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, "wa"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation("chat"); !errors.Is(err, ErrNotFound) {
		t.Errorf("conversation after account delete error = %v, want ErrNotFound", err)
	}
	if _, err := s.Message("incoming"); !errors.Is(err, ErrNotFound) {
		t.Errorf("message after account delete error = %v, want ErrNotFound", err)
	}
}

func TestMessageOrderingKeepsInsertionAndMillisecondOrder(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	seedConversation(t, s, "wa", "chat", "remote-chat", "Chat")
	for _, m := range []domain.Message{
		{ID: "fractional", ConversationID: "chat", Text: "before whole second", Created: 999},
		{ID: "whole", ConversationID: "chat", Text: "whole second", Created: 1000},
		{ID: "equal-1", ConversationID: "chat", Text: "equal first", Created: 2000},
		{ID: "equal-2", ConversationID: "chat", Text: "equal second", Created: 2000},
	} {
		if _, _, err := s.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	messages, _, err := s.Messages("chat", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(messages))
	for i, m := range messages {
		got[i] = m.ID
	}
	want := []string{"fractional", "whole", "equal-1", "equal-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message order = %v, want %v", got, want)
	}
}

func TestMessagesPagination(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	seedConversation(t, s, "wa", "chat", "remote-chat", "Chat")
	for i := 1; i <= 120; i++ {
		m := domain.Message{ID: fmt.Sprintf("m-%03d", i), ConversationID: "chat", Text: fmt.Sprintf("message %d", i), Created: int64(i)}
		if _, _, err := s.AddMessage(m); err != nil {
			t.Fatalf("AddMessage(%d): %v", i, err)
		}
	}

	page, hasMore, err := s.Messages("chat", "", 50)
	if err != nil || len(page) != 50 || !hasMore || page[0].ID != "m-071" || page[49].ID != "m-120" {
		t.Fatalf("first Messages() = len %d, hasMore %t, ids %v..%v, err %v", len(page), hasMore, page[0].ID, page[len(page)-1].ID, err)
	}
	page2, hasMore, err := s.Messages("chat", page[0].ID, 50)
	if err != nil || len(page2) != 50 || !hasMore || page2[0].ID != "m-021" || page2[49].ID != "m-070" {
		t.Fatalf("second Messages() = len %d, hasMore %t, err %v", len(page2), hasMore, err)
	}
	page3, hasMore, err := s.Messages("chat", page2[0].ID, 50)
	if err != nil || len(page3) != 20 || hasMore || page3[0].ID != "m-001" || page3[19].ID != "m-020" {
		t.Fatalf("third Messages() = len %d, hasMore %t, err %v", len(page3), hasMore, err)
	}
	if _, _, err := s.Messages("chat", "unknown", 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("Messages(unknown cursor) error = %v, want ErrNotFound", err)
	}
	if _, _, err := s.Messages("missing", "", 50); err != nil {
		t.Errorf("Messages(empty unknown conversation) error = %v, want empty result", err)
	}
	for _, limit := range []int{0, -1, 501} {
		page, _, err := s.Messages("chat", "", limit)
		if err != nil || len(page) != 50 {
			t.Errorf("Messages(limit=%d) len=%d err=%v, want default page of 50", limit, len(page), err)
		}
	}
}

func TestConversationsSearchLiteralWildcardsMatchAndOrdering(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	seedConversation(t, s, "wa", "title-hit", "r-title", "Release_100% Notes")
	seedConversation(t, s, "wa", "message-hit", "r-message", "A chat")
	seedConversation(t, s, "wa", "other", "r-other", "Other")
	for _, m := range []domain.Message{
		{ID: "m1", ConversationID: "message-hit", Text: "ticket 50% pending", Created: 10},
		{ID: "m2", ConversationID: "message-hit", Text: "newer ticket 50% resolved", Created: 20},
		{ID: "m3", ConversationID: "other", Text: "underscore _ literal", Created: 30},
	} {
		if _, _, err := s.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Conversations("ticket")
	if err != nil || len(got) != 1 || got[0].ID != "message-hit" || got[0].Match != "newer ticket 50% resolved" {
		t.Fatalf("Conversations(ticket) = %#v, %v", got, err)
	}
	got, err = s.Conversations("100%")
	if err != nil || len(got) != 1 || got[0].ID != "title-hit" {
		t.Fatalf("Conversations(100%%) = %#v, %v", got, err)
	}
	got, err = s.Conversations("_")
	if err != nil || len(got) != 2 {
		t.Fatalf("Conversations(_) = %#v, %v", got, err)
	}
	got, err = s.Conversations("")
	if err != nil || len(got) != 3 || got[0].ID != "other" || got[1].ID != "message-hit" {
		t.Fatalf("Conversations(empty) = %#v, %v", got, err)
	}
	got, err = s.Conversations("  RELEASE_100% NOTES  ")
	if err != nil || len(got) != 1 || got[0].ID != "title-hit" {
		t.Fatalf("case-insensitive title search = %#v, %v", got, err)
	}
}

func TestReadMutedUnreadAndMessageStatus(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	seedConversation(t, s, "wa", "chat", "remote-chat", "Chat")
	if _, _, err := s.AddMessage(domain.Message{ID: "in-1", ConversationID: "chat", Text: "one", Created: 1}); err != nil {
		t.Fatal(err)
	}
	changed, err := s.MarkRead("chat")
	if err != nil || !changed {
		t.Fatalf("first MarkRead() = %t, %v", changed, err)
	}
	changed, err = s.MarkRead("chat")
	if err != nil || changed {
		t.Fatalf("second MarkRead() = %t, %v", changed, err)
	}
	if _, err := s.MarkRead("unknown"); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkRead(unknown) error = %v, want ErrNotFound", err)
	}
	if err := s.SetMuted("chat", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddMessage(domain.Message{ID: "in-2", ConversationID: "chat", Text: "two", Created: 2}); err != nil {
		t.Fatal(err)
	}
	if total, err := s.UnreadTotal(); err != nil || total != 0 {
		t.Errorf("UnreadTotal() muted = %d, %v; want 0", total, err)
	}
	if err := s.SetMuted("chat", false); err != nil {
		t.Fatal(err)
	}
	if total, err := s.UnreadTotal(); err != nil || total != 1 {
		t.Errorf("UnreadTotal() unmuted = %d, %v; want 1", total, err)
	}
	if err := s.SetMuted("missing", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetMuted(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := s.UnreadTotal(); err != nil {
		t.Fatal(err)
	}

	if _, inserted, err := s.AddMessage(domain.Message{ID: "out-1", ConversationID: "chat", Text: "send", Outgoing: true, Created: 3}); err != nil || !inserted {
		t.Fatalf("AddMessage(outgoing) inserted=%t err=%v", inserted, err)
	}
	updated, changed, err := s.UpdateMessageStatus("out-1", domain.StatusSent)
	if err != nil || !changed || updated.Status != domain.StatusSent {
		t.Fatalf("pending -> sent = %#v, %t, %v", updated, changed, err)
	}
	updated, changed, err = s.UpdateMessageStatus("out-1", domain.StatusRead)
	if err != nil || !changed || updated.Status != domain.StatusRead {
		t.Fatalf("sent -> read = %#v, %t, %v", updated, changed, err)
	}
	updated, changed, err = s.UpdateMessageStatus("out-1", domain.StatusDelivered)
	if err != nil || changed || updated.Status != domain.StatusRead {
		t.Fatalf("read -> delivered = %#v, %t, %v", updated, changed, err)
	}
	if _, _, err := s.UpdateMessageStatus("missing", domain.StatusSent); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateMessageStatus(unknown) error = %v, want ErrNotFound", err)
	}
	if err := s.SetMessageRemoteID("missing", "remote"); err != nil {
		t.Errorf("SetMessageRemoteID(unknown) error = %v, want no-op", err)
	}
}

func TestContactsSearchCaseInsensitiveAndSorted(t *testing.T) {
	s, _ := openTestStore(t)
	seedAccount(t, s, "wa")
	for _, c := range []domain.Contact{
		{AccountID: "wa", RemoteID: "3", Name: "Zara Khan"},
		{AccountID: "wa", RemoteID: "2", Name: "alice Jones"},
		{AccountID: "wa", RemoteID: "1", Name: "Alice Smith"},
	} {
		if err := s.UpsertContact(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertContact(domain.Contact{AccountID: "wa", RemoteID: "1", Name: "Alice Cooper"}); err != nil {
		t.Fatal(err)
	}
	contact, err := s.Contact("wa", "1")
	if err != nil || contact.Name != "Alice Cooper" {
		t.Fatalf("Contact() = %#v, %v", contact, err)
	}
	if _, err := s.Contact("wa", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Contact(unknown) error = %v, want ErrNotFound", err)
	}
	contacts, err := s.Contacts("wa", "ALICE")
	if err != nil {
		t.Fatal(err)
	}
	if names := []string{contacts[0].Name, contacts[1].Name}; !reflect.DeepEqual(names, []string{"Alice Cooper", "alice Jones"}) {
		t.Errorf("contacts ordered = %v", names)
	}
	contacts, err = s.Contacts("wa", "")
	if err != nil || len(contacts) != 3 {
		t.Fatalf("Contacts(all) = %#v, %v", contacts, err)
	}
}
