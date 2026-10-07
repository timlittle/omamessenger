package server_test

import (
	"maps"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMethods_RoundTrip(t *testing.T) {
	t.Parallel()

	s := connect(t, true)
	ctx := t.Context()
	if err := s.store.UpsertContact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: "Ben"}); err != nil {
		t.Fatal(err)
	}

	accounts, err := call[[]domain.Account](t, s, "accounts.list", nil)
	if err != nil || len(accounts) != 1 {
		t.Errorf("accounts.list = %v, %v", accounts, err)
	}

	contacts, err := call[[]domain.Contact](t, s, "contacts.list", map[string]string{"accountId": "wa"})
	if err != nil || len(contacts) != 1 {
		t.Errorf("contacts.list = %v, %v", contacts, err)
	}

	opened, err := call[domain.Conversation](t, s, "conversations.open", map[string]string{"accountId": "wa", "contactId": "c1"})
	if err != nil || opened.Title != "Ben" {
		t.Errorf("conversations.open = %+v, %v", opened, err)
	}

	list, err := call[[]domain.Conversation](t, s, "conversations.list", map[string]string{"query": "ben"})
	if err != nil || len(list) != 1 {
		t.Errorf("conversations.list = %v, %v", list, err)
	}

	sent, err := call[domain.Message](t, s, "messages.send", map[string]string{"conversationId": "chat", "text": "hi"})
	if err != nil || sent.Status != domain.StatusPending {
		t.Errorf("messages.send = %+v, %v", sent, err)
	}

	page, err := call[struct {
		Messages []domain.Message `json:"messages"`
		HasMore  bool             `json:"hasMore"`
	}](t, s, "messages.list", map[string]string{"conversationId": "chat"})
	if err != nil || len(page.Messages) != 1 || page.HasMore {
		t.Errorf("messages.list = %+v, %v", page, err)
	}

	if _, err := call[domain.Message](t, s, "messages.retry", map[string]string{"messageId": sent.ID}); code(err) == 0 {
		t.Error("messages.retry of a pending message succeeded")
	}

	muted, err := call[domain.Conversation](t, s, "conversations.setMuted", map[string]any{"conversationId": "chat", "muted": true})
	if err != nil || !muted.Muted {
		t.Errorf("conversations.setMuted = %+v, %v", muted, err)
	}

	added, err := call[domain.Account](t, s, "accounts.add", map[string]any{"service": "telegram", "apiId": 1, "apiHash": "abc"})
	if err != nil || added.ID != "tg-new" {
		t.Errorf("accounts.add = %+v, %v", added, err)
	}

	want := map[string]string{"apiId": "1", "apiHash": "abc"}
	if got := s.accounts.lastOptions(); !maps.Equal(got, want) {
		t.Errorf("accounts.add options = %v, want %v", got, want)
	}

	for method, params := range map[string]any{
		"auth.submit":            map[string]string{"accountId": "tg-new", "step": "code", "value": "12345"},
		"accounts.remove":        map[string]string{"accountId": "tg-new"},
		"conversations.markRead": map[string]string{"conversationId": "chat"},
		"ui.setFocus":            map[string]any{"conversationId": "chat", "windowActive": true},
		"settings.apply":         map[string]bool{"notifications": true},
	} {
		if _, err := call[struct{}](t, s, method, params); err != nil {
			t.Errorf("%s = %v", method, err)
		}
	}
}

func TestAccountsAdd_AcceptsAGeneralOptionsObject(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[domain.Account](t, s, "accounts.add", map[string]any{
		"service": "telegram",
		"options": map[string]string{"apiId": "1", "apiHash": "abc"},
	}); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{"apiId": "1", "apiHash": "abc"}
	if got := s.accounts.lastOptions(); !maps.Equal(got, want) {
		t.Errorf("accounts.add options = %v, want %v", got, want)
	}
}

func TestAccountsAdd_ApiIDAndApiHashOverrideOptions(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[domain.Account](t, s, "accounts.add", map[string]any{
		"service": "telegram",
		"apiId":   2,
		"apiHash": "own",
		"options": map[string]string{"apiId": "1", "apiHash": "abc"},
	}); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{"apiId": "2", "apiHash": "own"}
	if got := s.accounts.lastOptions(); !maps.Equal(got, want) {
		t.Errorf("accounts.add options = %v, want %v", got, want)
	}
}
