package store_test

// conversations_sanitize_test.go confirms a conversation's own
// sender-controlled display text - its Title, its Preview, its
// PreviewSender and a search result's MatchSender - is capped on every
// read, even when the row was stored uncapped by a release before
// domain.MaxDisplayNameLength or domain.MaxPreviewLength existed, or by
// any other path that skips the app layer's own CapDisplayName or
// CapPreview call. See store.scanConversationWith.

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConversation_CapsAStoredTitleOnRead confirms a conversation's own
// Title is capped on every read, the same defence store.scanMessage
// gives a legacy sender name.
func TestConversation_CapsAStoredTitleOnRead(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	huge := strings.Repeat("x", domain.MaxDisplayNameLength+1)
	created, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: "remote", Title: huge, Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	got, err := s.Conversation(ctx, created.ID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if domain.UTF16Len(got.Title) != domain.MaxDisplayNameLength {
		t.Errorf("Title length = %d, want %d", domain.UTF16Len(got.Title), domain.MaxDisplayNameLength)
	}
}

// TestConversation_CapsAStoredPreviewSenderOnRead confirms a
// conversation's own PreviewSender, set from a message's SenderName
// stored uncapped by a release before that cap existed, is capped on
// every read.
func TestConversation_CapsAStoredPreviewSenderOnRead(t *testing.T) {
	t.Parallel()

	s, conv := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("x", domain.MaxDisplayNameLength+1)
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: conv.ID, Text: "hi", Created: 1, SenderName: huge})

	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if domain.UTF16Len(got.PreviewSender) != domain.MaxDisplayNameLength {
		t.Errorf("PreviewSender length = %d, want %d", domain.UTF16Len(got.PreviewSender), domain.MaxDisplayNameLength)
	}
}

// TestConversation_CapsAStoredPreviewOnRead confirms a conversation's
// own Preview is capped on every read, even when it was set to a
// message's full, unbounded text by a path that does not go through
// bumpConversation's own domain.CapPreview call: refreshAfterDeletion
// falls a conversation's preview back to its newest remaining
// message's raw text column, the same way a release before
// domain.MaxPreviewLength existed stored bumpConversation's own preview
// uncapped.
func TestConversation_CapsAStoredPreviewOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("a", domain.MaxPreviewLength+1000)
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: huge, Created: 1},
		domain.Message{ID: "m2", ConversationID: "chat", RemoteID: "2", Text: "short", Created: 2},
	)

	if _, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, []string{"2"}); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}

	got, err := s.Conversation(ctx, "chat")
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if n := domain.UTF16Len(got.Preview); n != domain.MaxPreviewLength {
		t.Errorf("Preview length = %d, want %d", n, domain.MaxPreviewLength)
	}
}

// TestConversations_CapsMatchSenderOnRead confirms a search result's
// MatchSender, read from a matched message's own sender name stored
// uncapped by a release before domain.MaxDisplayNameLength existed, is
// capped the same way Conversation's PreviewSender now is.
func TestConversations_CapsMatchSenderOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	huge := strings.Repeat("x", domain.MaxDisplayNameLength+1)
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", SenderName: huge, Text: "ticket opened", Created: 1})

	got, err := s.Conversations(t.Context(), "ticket")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Conversations(ticket) = %v, want one match", got)
	}
	if domain.UTF16Len(got[0].MatchSender) != domain.MaxDisplayNameLength {
		t.Errorf("MatchSender length = %d, want %d", domain.UTF16Len(got[0].MatchSender), domain.MaxDisplayNameLength)
	}
}
