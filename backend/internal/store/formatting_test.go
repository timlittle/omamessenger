package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestAddMessage_StoresAndReadsBackFormatting(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	spans := []domain.Span{{Kind: domain.SpanLink, Start: 0, End: 4, URL: "https://example.com"}}

	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "link here", Created: 1,
		Formatting: spans,
	})

	got, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(got.Formatting, spans) {
		t.Errorf("stored message = %+v, %v; want formatting %+v", got, err, spans)
	}
}

func TestAddMessage_LeavesFormattingEmptyByDefault(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "hi", Created: 1})

	got, err := s.Message(ctx, "m1")
	if err != nil || len(got.Formatting) != 0 {
		t.Errorf("stored message = %+v, %v; want no formatting", got, err)
	}
}

func TestEditMessage_UpdatesFormatting(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	spans := []domain.Span{{Kind: domain.SpanBold, Start: 0, End: 2}}
	got, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{Text: "hi there", Formatting: spans})
	if err != nil || !found || !slices.Equal(got.Formatting, spans) {
		t.Fatalf("EditMessage formatting = %+v, found %t, %v", got, found, err)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || !slices.Equal(stored.Formatting, spans) {
		t.Errorf("stored message = %+v, %v", stored, err)
	}
}
