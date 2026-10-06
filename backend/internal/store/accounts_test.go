package store_test

import (
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestUpsertAccount_RejectsInvalidAccounts(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	tests := []struct {
		name    string
		account domain.Account
	}{
		{"missing id", domain.Account{Service: domain.ServiceWhatsApp, Name: "Name"}},
		{"missing name", domain.Account{ID: "id", Service: domain.ServiceWhatsApp}},
		{"unsupported service", domain.Account{ID: "id", Service: "signal", Name: "Name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.UpsertAccount(t.Context(), tt.account); !errors.Is(err, store.ErrInvalidAccount) {
				t.Fatalf("UpsertAccount = %v, want ErrInvalidAccount", err)
			}
		})
	}
}

func TestUpsertAccount_RenamesButKeepsStatus(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	got, err := s.Account(ctx, "wa")
	if err != nil || got.Status != domain.AccountOffline {
		t.Fatalf("new account = %+v, %v; want offline", got, err)
	}

	renamed := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Renamed", Status: domain.AccountConnected}
	if err := s.UpsertAccount(ctx, renamed); err != nil {
		t.Fatal(err)
	}

	got, err = s.Account(ctx, "wa")
	if err != nil || got.Name != "Renamed" || got.Status != domain.AccountOffline {
		t.Fatalf("renamed account = %+v, %v", got, err)
	}
}

func TestSetAccountStatus(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	got, err := s.SetAccountStatus(ctx, "wa", domain.AccountConnected, "Ready")
	if err != nil || got.Status != domain.AccountConnected || got.Detail != "Ready" {
		t.Fatalf("SetAccountStatus = %+v, %v", got, err)
	}

	if _, err := s.SetAccountStatus(ctx, "missing", domain.AccountError, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetAccountStatus(missing) = %v, want ErrNotFound", err)
	}
}

func TestAccounts_ListsAndFinds(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	accounts, err := s.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa" {
		t.Fatalf("Accounts = %+v, %v", accounts, err)
	}

	if _, err := s.Account(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Account(missing) = %v, want ErrNotFound", err)
	}
}

func TestDeleteAccount_RemovesEverythingOfTheAccount(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addAccount(t, s, "keep")
	addConversation(t, s, "wa", "chat", "Chat")
	addConversation(t, s, "keep", "other", "Other")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", Text: "hi", Created: 1})
	if err := s.UpsertContact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: "Ben"}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteAccount(ctx, "wa"); err != nil {
		t.Fatal(err)
	}

	for name, err := range map[string]error{
		"account":      func() error { _, err := s.Account(ctx, "wa"); return err }(),
		"conversation": func() error { _, err := s.Conversation(ctx, "chat"); return err }(),
		"message":      func() error { _, err := s.Message(ctx, "m1"); return err }(),
		"contact":      func() error { _, err := s.Contact(ctx, "wa", "c1"); return err }(),
	} {
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s after deleting the account: %v, want ErrNotFound", name, err)
		}
	}

	if _, err := s.Conversation(ctx, "other"); err != nil {
		t.Errorf("another account's conversation went too: %v", err)
	}

	if err := s.DeleteAccount(ctx, "wa"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleting it twice = %v, want ErrNotFound", err)
	}
}
