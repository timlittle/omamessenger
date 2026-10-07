package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestAddAccount_StartsATelegramAccount(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	account, err := f.commands.AddAccount(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, Options: map[string]string{"apiId": "12345", "apiHash": "abc"}})
	if err != nil || account.ID != "tg-new" {
		t.Fatalf("AddAccount = %+v, %v", account, err)
	}

	if !slices.Equal(f.published.take(), []string{app.EventAccountUpdated}) {
		t.Error("adding an account did not publish it")
	}
}

func TestAddAccount_LeavesSetupOptionsToTheRegistry(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	if _, err := f.commands.AddAccount(t.Context(), app.NewAccount{Service: domain.ServiceTelegram}); err != nil {
		t.Fatal(err)
	}

	if got := f.accounts.added; len(got) != 1 || got[0].Options != nil {
		t.Errorf("added %+v, want one account passed through with no options", got)
	}
}

func TestAddAccount_MapsRegistryFailuresToInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	for name, registryErr := range map[string]error{
		"unknown service": connector.ErrUnknownProvider,
		"invalid setup":   connector.ErrInvalidSetup,
	} {
		f.accounts.err = registryErr
		if _, err := f.commands.AddAccount(t.Context(), app.NewAccount{Service: domain.ServiceTelegram}); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: AddAccount = %v, want ErrInvalidInput", name, err)
		}
	}

	if len(f.accounts.added) != 0 {
		t.Errorf("rejected accounts were added: %+v", f.accounts.added)
	}
}

func TestAddAccount_ReportsFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.accounts.err = errors.New("disk full")

	if _, err := f.commands.AddAccount(t.Context(), app.NewAccount{Service: domain.ServiceTelegram, Options: map[string]string{"apiId": "1", "apiHash": "abc"}}); err == nil {
		t.Error("AddAccount succeeded although adding failed")
	}

	if len(f.published.take()) != 0 {
		t.Error("a failed add published an account")
	}
}

func TestServices_IsEmptyWithoutALister(t *testing.T) {
	t.Parallel()

	commands, _ := app.New(app.Deps{Accounts: noListerAccounts{}})
	if got := commands.Services(); got != nil {
		t.Errorf("Services() = %+v, want none", got)
	}
}

func TestServices_ListsWhatTheRegistryOffers(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	want := []domain.Service{{ID: domain.ServiceTelegram, Name: "Telegram"}}
	f.accounts.services = want

	if got := f.commands.Services(); !slices.Equal(got, want) {
		t.Errorf("Services() = %+v, want %+v", got, want)
	}
}

func TestRemoveAccount_DeletesItsChatsAndUpdatesUnread(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 1)
	f.published.take()

	if err := f.commands.RemoveAccount(ctx, "wa"); err != nil {
		t.Fatal(err)
	}

	want := []string{app.EventAccountRemoved, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	if _, err := f.store.Conversation(ctx, chat.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("the account's conversation survived: %v", err)
	}

	if err := f.commands.RemoveAccount(ctx, "wa"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("removing it twice = %v, want ErrNotFound", err)
	}
}

func TestSubmitAuth_PassesTrimmedAnswersOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	if err := f.commands.SubmitAuth(t.Context(), "tg", "code", " 12345 "); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(f.signIn.answers, []string{"tg code=12345"}) {
		t.Errorf("answers = %v", f.signIn.answers)
	}

	for _, bad := range [][2]string{{"qr", "x"}, {"code", "  "}} {
		if err := f.commands.SubmitAuth(t.Context(), "tg", bad[0], bad[1]); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("SubmitAuth(%q, %q) = %v, want ErrInvalidInput", bad[0], bad[1], err)
		}
	}
}

func TestAuthStep_IsPublished(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.ingest.AuthStep(t.Context(), "tg", connector.AuthStep{Kind: "code", Hint: "That code did not work."})

	want := app.AuthStep{AccountID: "tg", Kind: "code", Hint: "That code did not work."}
	if got := f.published.last(); got != want {
		t.Errorf("published %+v, want %+v", got, want)
	}
}
