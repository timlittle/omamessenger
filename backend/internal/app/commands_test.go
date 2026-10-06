package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestAccounts(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	accounts, err := f.commands.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa" {
		t.Fatalf("Accounts = %+v, %v", accounts, err)
	}
}

func TestContacts(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	if err := f.store.UpsertContact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: "New Contact"}); err != nil {
		t.Fatal(err)
	}

	contacts, err := f.commands.Contacts(ctx, "wa", "contact")
	if err != nil || len(contacts) != 1 || contacts[0].Name != "New Contact" {
		t.Fatalf("Contacts = %+v, %v", contacts, err)
	}

	if _, err := f.commands.Contacts(ctx, "", ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Contacts(no account) = %v, want ErrInvalidInput", err)
	}

	if _, err := f.commands.Contacts(ctx, "missing", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Contacts(unknown account) = %v, want ErrNotFound", err)
	}
}

func TestSetFocus_ChecksTheConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	for _, id := range []string{"chat", ""} {
		if err := f.commands.SetFocus(t.Context(), id, true); err != nil {
			t.Errorf("SetFocus(%q) = %v", id, err)
		}
	}

	if err := f.commands.SetFocus(t.Context(), "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetFocus(missing) = %v, want ErrNotFound", err)
	}
}

func TestInject_ReturnsTheStoredMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, true)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.injector.inject = func(ctx context.Context, remoteID string) (domain.Message, error) {
		m := incoming("injected-1", "hello")
		f.ingest.Incoming(ctx, "wa", remoteID, m)

		return m, nil
	}

	got, err := f.commands.Inject(t.Context(), chat.ID)
	if err != nil || got.ConversationID != chat.ID || got.ID == "" {
		t.Fatalf("Inject = %+v, %v", got, err)
	}

	if _, err := f.commands.Inject(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Inject(missing) = %v, want ErrNotFound", err)
	}

	f.injector.inject = func(context.Context, string) (domain.Message, error) {
		return domain.Message{}, errors.New("stopped")
	}
	if _, err := f.commands.Inject(t.Context(), chat.ID); err == nil {
		t.Error("Inject succeeded when the fake failed")
	}
}

func TestInject_RefusedWithoutFakes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	if f.commands.Faked() {
		t.Error("Faked = true without fake connectors")
	}

	if _, err := f.commands.Inject(t.Context(), "chat"); !errors.Is(err, app.ErrNoFake) {
		t.Errorf("Inject = %v, want ErrNoFake", err)
	}
}
