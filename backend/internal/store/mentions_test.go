package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestAddMessage_StoresAndReadsBackMentions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	mentions := []domain.Mention{{UserID: "wa-2", Name: "Priya", Offset: 0, Length: 5}}

	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "@Priya hi", Created: 1,
		Mentions: mentions, MentionsMe: true,
	})

	got, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(got.Mentions, mentions) || !got.MentionsMe {
		t.Errorf("stored message = %+v, %v; want mentions %+v and mentionsMe true", got, err, mentions)
	}
}

func TestAddMessage_LeavesMentionsEmptyByDefault(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "hi", Created: 1})

	got, err := s.Message(ctx, "m1")
	if err != nil || len(got.Mentions) != 0 || got.MentionsMe {
		t.Errorf("stored message = %+v, %v; want no mentions", got, err)
	}
}

func TestEditMessage_UpdatesMentions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	mentions := []domain.Mention{{UserID: "wa-2", Name: "Priya", Offset: 0, Length: 5}}
	got, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{Text: "@Priya hi", Mentions: mentions, MentionsMe: true})
	if err != nil || !found || !slices.Equal(got.Mentions, mentions) || !got.MentionsMe {
		t.Fatalf("EditMessage mentions = %+v, found %t, %v", got, found, err)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(stored.Mentions, mentions) || !stored.MentionsMe {
		t.Errorf("stored message = %+v, %v", stored, err)
	}
}
