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
