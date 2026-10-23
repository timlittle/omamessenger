package app_test

import (
	"context"
	"errors"
	"maps"
	"slices"
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

// TestUnreadByService_GroupsByService confirms Commands.UnreadByService
// - what server.hello's per-service breakdown reads - matches the same
// grouping TestUnreadChanged_BreaksDownUnreadByService already proves
// for the unread.changed event's own ByService field, called directly
// rather than only read back off a published event.
func TestUnreadByService_GroupsByService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if err := f.store.UpsertAccount(ctx, domain.Account{ID: "tg", Service: domain.ServiceTelegram, Name: "tg"}); err != nil {
		t.Fatal(err)
	}
	tgChat, _, err := f.store.EnsureConversation(ctx, domain.Conversation{
		ID: "tg-chat", AccountID: "tg", RemoteID: "r-tg-chat", Title: "TG Chat", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	f.ingest.Unread(ctx, "wa", chat.RemoteID, 2)
	f.ingest.Unread(ctx, "tg", tgChat.RemoteID, 1)

	got := f.commands.UnreadByService(ctx)
	want := map[string]int{domain.ServiceWhatsApp: 2, domain.ServiceTelegram: 1}
	if !maps.Equal(got, want) {
		t.Errorf("UnreadByService() = %v, want %v", got, want)
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

// TestCheckSpelling_EnabledByDefault finds a misspelling without any
// settings.apply call, since spell checking starts at the same default
// as every other setting (see app.New).
func TestCheckSpelling_EnabledByDefault(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	got, err := f.commands.CheckSpelling(t.Context(), "this is wrongg")
	if err != nil {
		t.Fatalf("CheckSpelling: %v", err)
	}
	if len(got) != 1 || got[0].Word != "wrongg" {
		t.Fatalf("CheckSpelling = %+v, want one misspelling of %q", got, "wrongg")
	}
}

// TestApplySettings_TurnsSpellCheckOff disables spell checking through
// the same settings path the UI uses.
func TestApplySettings_TurnsSpellCheckOff(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	f.commands.ApplySettings(app.Settings{SpellCheck: false, SpellLanguage: "auto"})

	got, err := f.commands.CheckSpelling(t.Context(), "wrongg")
	if err != nil {
		t.Fatalf("CheckSpelling: %v", err)
	}
	if got != nil {
		t.Errorf("CheckSpelling with spell check off = %+v, want nil", got)
	}
}

// TestSuggestSpelling_AndAddSpellingWord round-trips a word through
// Suggest and AddWord.
func TestSuggestSpelling_AndAddSpellingWord(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	suggestions, err := f.commands.SuggestSpelling(ctx, "colr")
	if err != nil {
		t.Fatalf("SuggestSpelling: %v", err)
	}
	if !slices.Contains(suggestions, "color") {
		t.Errorf("SuggestSpelling(colr) = %v, want it to contain %q", suggestions, "color")
	}

	if err := f.commands.AddSpellingWord(ctx, "omamessenger"); err != nil {
		t.Fatalf("AddSpellingWord: %v", err)
	}
	got, err := f.commands.CheckSpelling(ctx, "omamessenger")
	if err != nil {
		t.Fatalf("CheckSpelling: %v", err)
	}
	if got != nil {
		t.Errorf("CheckSpelling(omamessenger) after AddSpellingWord = %+v, want nil", got)
	}
}

// TestAddSpellingWord_RejectsEmpty reports invalid input rather than
// silently adding nothing.
func TestAddSpellingWord_RejectsEmpty(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.AddSpellingWord(t.Context(), ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("AddSpellingWord(\"\") = %v, want ErrInvalidInput", err)
	}
}

// TestSpellMethods_WithoutASpellChecker no-op rather than panic when no
// spell.Checker was wired in (see appOver, which leaves Deps.Spell
// unset, the same as a build that chose not to provide one).
func TestSpellMethods_WithoutASpellChecker(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	commands, _, _, _ := appOver(t, f.store)
	ctx := t.Context()

	commands.ApplySettings(app.Settings{SpellCheck: true, SpellLanguage: "auto"})

	got, err := commands.CheckSpelling(ctx, "wrongg")
	if err != nil || got != nil {
		t.Errorf("CheckSpelling without a spell checker = %+v, %v, want nil, nil", got, err)
	}

	suggestions, err := commands.SuggestSpelling(ctx, "colr")
	if err != nil || suggestions != nil {
		t.Errorf("SuggestSpelling without a spell checker = %v, %v, want nil, nil", suggestions, err)
	}

	if err := commands.AddSpellingWord(ctx, "omamessenger"); err != nil {
		t.Errorf("AddSpellingWord without a spell checker = %v, want nil", err)
	}
}
