package server_test

import (
	"bytes"
	"image"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/doctor"
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

	if err := s.store.SetMessageRemoteID(ctx, sent.ID, "r1"); err != nil {
		t.Fatal(err)
	}

	if _, err := call[domain.Message](t, s, "messages.react", map[string]string{"messageId": sent.ID, "emoji": "👍"}); err != nil {
		t.Errorf("messages.react = %v", err)
	}

	if _, err := call[struct{}](t, s, "messages.delete", map[string]any{
		"conversationId": "chat", "messageIds": []string{sent.ID}, "forEveryone": true,
	}); err != nil {
		t.Errorf("messages.delete = %v", err)
	}
	if _, err := s.store.Message(ctx, sent.ID); err == nil {
		t.Error("message still stored after messages.delete")
	}

	muted, err := call[domain.Conversation](t, s, "conversations.setMuted", map[string]any{"conversationId": "chat", "muted": true})
	if err != nil || !muted.Muted {
		t.Errorf("conversations.setMuted = %+v, %v", muted, err)
	}

	pinned, err := call[domain.Conversation](t, s, "conversations.setPinned", map[string]any{"conversationId": "chat", "pinned": true})
	if err != nil || !pinned.Pinned {
		t.Errorf("conversations.setPinned = %+v, %v", pinned, err)
	}

	archived, err := call[domain.Conversation](t, s, "conversations.setArchived", map[string]any{"conversationId": "chat", "archived": true})
	if err != nil || !archived.Archived {
		t.Errorf("conversations.setArchived = %+v, %v", archived, err)
	}

	hidden, err := call[domain.Conversation](t, s, "conversations.setHidden", map[string]any{"conversationId": "chat", "hidden": true})
	if err != nil || !hidden.Hidden {
		t.Errorf("conversations.setHidden = %+v, %v", hidden, err)
	}

	snoozed, err := call[domain.Conversation](t, s, "conversations.setReminder", map[string]any{"conversationId": "chat", "at": 5000})
	if err != nil || snoozed.ReminderAt != 5000 {
		t.Errorf("conversations.setReminder = %+v, %v", snoozed, err)
	}

	unsnoozed, err := call[domain.Conversation](t, s, "conversations.setReminder", map[string]any{"conversationId": "chat", "at": nil})
	if err != nil || unsnoozed.ReminderAt != 0 {
		t.Errorf("conversations.setReminder(null) = %+v, %v", unsnoozed, err)
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

func TestMessagesSend_WithReplyToQuotesTheOriginalMessage(t *testing.T) {
	t.Parallel()

	s := connect(t, true)

	original, err := call[domain.Message](t, s, "messages.send", map[string]string{"conversationId": "chat", "text": "hi"})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := call[domain.Message](t, s, "messages.send", map[string]string{"conversationId": "chat", "text": "sure", "replyTo": original.ID})
	if err != nil || reply.ReplyTo == nil || reply.ReplyTo.Text != "hi" {
		t.Errorf("messages.send with replyTo = %+v, %v", reply, err)
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

func TestMessagesSend_AcceptsAnAttachment(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	sent, err := call[domain.Message](t, s, "messages.send", map[string]any{
		"conversationId": "chat", "text": "see attached",
		"attachment": map[string]string{"path": path},
	})
	if err != nil || sent.Media == nil || sent.Media.Kind != domain.MediaFile || sent.Text != "see attached" {
		t.Errorf("messages.send with an attachment = %+v, %v", sent, err)
	}
}

func TestMessagesSend_WithAttachmentAndReplyToTogether(t *testing.T) {
	t.Parallel()

	s := connect(t, true)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	original, err := call[domain.Message](t, s, "messages.send", map[string]string{"conversationId": "chat", "text": "hi"})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := call[domain.Message](t, s, "messages.send", map[string]any{
		"conversationId": "chat", "text": "see attached",
		"attachment": map[string]string{"path": path},
		"replyTo":    original.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Media == nil || reply.Media.Kind != domain.MediaFile {
		t.Errorf("reply.Media = %+v, want the attachment kept alongside the reply", reply.Media)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.Text != "hi" {
		t.Errorf("reply.ReplyTo = %+v, want it to quote the original message", reply.ReplyTo)
	}
}

// TestHelperDoctor_ReturnsOneCheckPerConcernIncludingTheSignedInAccount
// confirms the method reaches app.Commands.Doctor and decodes back into
// the same shape the UI's command palette would read.
func TestHelperDoctor_ReturnsOneCheckPerConcernIncludingTheSignedInAccount(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	report, err := call[doctor.Report](t, s, "helper.doctor", nil)
	if err != nil || len(report.Checks) == 0 {
		t.Fatalf("helper.doctor = %+v, %v", report, err)
	}

	found := false
	for _, c := range report.Checks {
		if c.Name == "Account (whatsapp)" {
			found = true
		}
	}
	if !found {
		t.Errorf("checks = %+v, want one for the signed-in whatsapp account", report.Checks)
	}
}

func TestMediaPaste_ReturnsTheClipboardImage(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.clipboard.types = []string{"image/png"}
	s.clipboard.data = map[string][]byte{"image/png": pngBytes(t, 2, 2)}

	pasted, err := call[struct {
		Path   string `json:"path"`
		Kind   string `json:"kind"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}](t, s, "media.paste", nil)
	if err != nil || pasted.Kind != domain.MediaPhoto || pasted.Width != 2 || pasted.Height != 2 || pasted.Path == "" {
		t.Errorf("media.paste = %+v, %v", pasted, err)
	}
}

func TestMediaPaste_RejectsAnEmptyClipboard(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.clipboard.types = []string{"text/plain"}

	if _, err := call[struct{}](t, s, "media.paste", nil); code(err) != -32602 {
		t.Errorf("media.paste(no image) code = %d, want invalid params", code(err))
	}
}

// pngBytes encodes a solid image of the given size as a PNG.
func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}
