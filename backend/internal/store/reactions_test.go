package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestAddMessage_StoresReactions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	reactions := []domain.Reaction{{Emoji: "🔥", Count: 3, Mine: true}}

	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "hi", Created: 1, Reactions: reactions})

	got, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(got.Reactions, reactions) {
		t.Errorf("stored reactions = %+v, %v; want %+v", got.Reactions, err, reactions)
	}
}

func TestAddMessage_LeavesReactionsEmptyByDefault(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "hi", Created: 1})

	got, err := s.Message(ctx, "m1")
	if err != nil || len(got.Reactions) != 0 {
		t.Errorf("stored reactions = %+v, %v; want none", got.Reactions, err)
	}
}

func TestEditMessage_UpdatesReactions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	reactions := []domain.Reaction{{Emoji: "👍", Count: 1, Mine: true}}
	got, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{Text: "hi", Reactions: reactions})
	if err != nil || !found || !slices.Equal(got.Reactions, reactions) {
		t.Fatalf("EditMessage reactions = %+v, found %t, %v", got.Reactions, found, err)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(stored.Reactions, reactions) {
		t.Errorf("stored reactions = %+v, %v", stored.Reactions, err)
	}
}

func TestSetReactions_UpdatesWithoutTouchingTextOrMedia(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	link := &domain.Media{Kind: domain.MediaLink, URL: "https://x.io"}
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Media: link, Created: 1})

	reactions := []domain.Reaction{{Emoji: "❤️", Count: 2, Mine: false}}
	got, found, err := s.SetReactions(ctx, "chat", "r1", reactions)
	if err != nil || !found || !slices.Equal(got.Reactions, reactions) {
		t.Fatalf("SetReactions = %+v, found %t, %v", got.Reactions, found, err)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || stored.Text != "hi" || stored.Media == nil || !slices.Equal(stored.Reactions, reactions) {
		t.Errorf("stored message = %+v, %v; want text and media untouched", stored, err)
	}
}

func TestSetReactions_IgnoresAMessageThatIsNotStored(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	got, found, err := s.SetReactions(ctx, "chat", "missing", []domain.Reaction{{Emoji: "👍", Count: 1}})
	if err != nil || found || got.ID != "" {
		t.Fatalf("SetReactions(missing) = %+v, found %t, %v; want ignored", got, found, err)
	}
}

func TestSetReactions_ClearsReactionsWhenEmpty(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	if _, _, err := s.SetReactions(ctx, "chat", "r1", []domain.Reaction{{Emoji: "👍", Count: 1, Mine: true}}); err != nil {
		t.Fatal(err)
	}

	got, found, err := s.SetReactions(ctx, "chat", "r1", nil)
	if err != nil || !found || len(got.Reactions) != 0 {
		t.Fatalf("SetReactions(nil) = %+v, found %t, %v; want no reactions", got.Reactions, found, err)
	}
}
