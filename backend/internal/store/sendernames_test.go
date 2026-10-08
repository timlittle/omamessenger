package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestRefreshSenderName_UpdatesTheGroupPreviewOnceTheSenderResolves
// reproduces a group whose preview still names its newest message's
// sender by a stale, generic label, because a connector only learns
// that sender's real name after the message was already stored (see
// bumpConversation, which sets preview_sender once and never again).
// RefreshSenderName is what a connector calls once a contact, push or
// business name resolves, so the group's preview catches up without
// waiting for another message to arrive.
func TestRefreshSenderName_UpdatesTheGroupPreviewOnceTheSenderResolves(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	group := addConversation(t, s, "wa", "group", "Weekend plans")
	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: group.ID, RemoteID: "r1",
		SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100,
	})

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair")
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if !slices.Contains(changed, group.ID) {
		t.Errorf("RefreshSenderName changed = %v, want it to include %q", changed, group.ID)
	}

	updated, err := s.Conversation(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviewSender != "Priya Nair" {
		t.Errorf("conversation.PreviewSender = %q, want %q", updated.PreviewSender, "Priya Nair")
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "Priya Nair" {
		t.Errorf("message.SenderName = %q, want it refreshed to %q too", stored.SenderName, "Priya Nair")
	}
}

// TestRefreshSenderName_LeavesAnOlderMessagesPreviewAlone confirms that
// when the sender being refreshed is not who sent the conversation's
// newest message, its own sender name is still corrected, but the
// conversation's preview, which already names whoever sent the newest
// message, is left exactly as it was.
func TestRefreshSenderName_LeavesAnOlderMessagesPreviewAlone(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	group := addConversation(t, s, "wa", "group", "Weekend plans")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: group.ID, RemoteID: "r1", SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100},
		domain.Message{ID: "m2", ConversationID: group.ID, RemoteID: "r2", SenderID: "r-sam", SenderName: "Sam", Text: "Sounds good", Created: 200},
	)

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair")
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("RefreshSenderName changed = %v, want no conversation whose preview needed to change", changed)
	}

	updated, err := s.Conversation(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviewSender != "Sam" {
		t.Errorf("conversation.PreviewSender = %q, want the newest message's own sender left alone", updated.PreviewSender)
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "Priya Nair" {
		t.Errorf("message.SenderName = %q, want the older message's own sender name refreshed regardless", stored.SenderName)
	}
}

// TestRefreshSenderName_IsScopedToOneAccount confirms a sender id that
// happens to match one used by a different account's conversation is
// never touched: remote ids are only unique within a service account.
func TestRefreshSenderName_IsScopedToOneAccount(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addAccount(t, s, "wa2")
	group := addConversation(t, s, "wa2", "group", "Other account's group")
	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: group.ID, RemoteID: "r1",
		SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100,
	})

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair")
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("RefreshSenderName changed = %v, want another account's conversation left untouched", changed)
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "WhatsApp user" {
		t.Errorf("message.SenderName = %q, want another account's message left untouched", stored.SenderName)
	}
}
