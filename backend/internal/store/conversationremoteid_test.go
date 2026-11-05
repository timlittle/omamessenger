package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestUpgradeConversationRemoteID_RenamesInPlace proves the simple case
// a connector hits once it resolves a better id for a conversation it
// already reported under a weaker one, such as Telegram learning a
// placeholder peer's real access hash: nothing yet exists under the
// new id, so the existing conversation just gets a new remote id,
// keeping its local id and messages.
func TestUpgradeConversationRemoteID_RenamesInPlace(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: placeholder.ID, Text: "hi", Created: 1})

	upgraded, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890")
	if err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	if upgraded.ID != placeholder.ID || upgraded.RemoteID != "user:42:67890" {
		t.Errorf("upgraded = %+v, want id %q and remote id %q", upgraded, placeholder.ID, "user:42:67890")
	}

	if _, err := s.ConversationByRemote(ctx, "tg", "user:42:0"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConversationByRemote(old id) = %v, want ErrNotFound", err)
	}

	messages, _, err := s.Messages(ctx, placeholder.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "m1" {
		t.Errorf("Messages after rename = %+v, %v; want the one message kept", messages, err)
	}
}

// TestUpgradeConversationRemoteID_MergesIntoTheExistingConversation
// proves the case this exists for: Telegram resolves a placeholder's
// real id to one a conversation already exists under - the two must
// never end up as duplicates. The placeholder's own message moves
// into the real conversation, which keeps its own id, and the
// placeholder is removed.
func TestUpgradeConversationRemoteID_MergesIntoTheExistingConversation(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	real, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(real): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "earlier", ConversationID: real.ID, Text: "hey", Created: 1})

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(placeholder): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "stray", ConversationID: placeholder.ID, Text: "hi", Created: 2})

	merged, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890")
	if err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	if merged.ID != real.ID || merged.RemoteID != "user:42:67890" {
		t.Errorf("merged = %+v, want the real conversation %q kept", merged, real.ID)
	}

	if _, err := s.Conversation(ctx, placeholder.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Conversation(placeholder) = %v, want ErrNotFound, placeholder must be removed", err)
	}

	messages, _, err := s.Messages(ctx, real.ID, "", 10)
	if err != nil || len(ids(messages)) != 2 || !slices.Contains(ids(messages), "earlier") || !slices.Contains(ids(messages), "stray") {
		t.Errorf("Messages after merge = %+v, %v; want both earlier and stray kept under %q", messages, err, real.ID)
	}
}

// TestUpgradeConversationRemoteID_DropsAMessageTheRealConversationAlreadyHas
// proves the merge never duplicates a message the real conversation
// already stored under the same remote id as one the placeholder also
// has - the placeholder's own copy is dropped, not kept as a second
// row.
func TestUpgradeConversationRemoteID_DropsAMessageTheRealConversationAlreadyHas(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	real, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(real): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "kept", ConversationID: real.ID, RemoteID: "9", Text: "hey", Created: 1})

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(placeholder): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "duplicate", ConversationID: placeholder.ID, RemoteID: "9", Text: "hey", Created: 1})

	if _, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890"); err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	messages, _, err := s.Messages(ctx, real.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "kept" {
		t.Errorf("Messages after merge = %+v, %v; want only the real conversation's own message kept", messages, err)
	}
}

// TestUpgradeConversationRemoteID_ReportsNotFoundForAnUnknownOldID
// proves the common, harmless case: nothing is stored under oldRemoteID
// at all, such as a connector's own idempotent self-heal running
// against a peer that was never actually a placeholder. Nothing
// changes and the caller sees domain.ErrNotFound, the same as any
// other lookup by an id that does not exist.
func TestUpgradeConversationRemoteID_ReportsNotFoundForAnUnknownOldID(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	if _, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpgradeConversationRemoteID(unknown) = %v, want ErrNotFound", err)
	}
}

// TestUpgradeConversationRemoteID_SameIDIsANoOp proves that upgrading a
// conversation to the remote id it already has changes nothing and
// still returns it, rather than tripping over renaming an id onto
// itself.
func TestUpgradeConversationRemoteID_SameIDIsANoOp(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	conv, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	got, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:67890", "user:42:67890")
	if err != nil || got.ID != conv.ID {
		t.Errorf("UpgradeConversationRemoteID(same id) = %+v, %v; want %q unchanged", got, err, conv.ID)
	}
}
