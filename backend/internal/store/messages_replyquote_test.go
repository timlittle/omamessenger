package store_test

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMessage_RecapsAStoredOversizedReplyQuoteText confirms a reply
// quote's own Text, stored at full length because resolveReply skips
// shortening it whenever the incoming reply already carries a sender
// id, a sender name and text (see resolveReply's own comment) - the
// same shape a row an older release, before domain.Excerpt was applied
// on write, could have stored too - is cut to domain.ExcerptLength on
// every read, the same defence TestMessage_CapsAStoredStickerEmoji
// already gives a legacy sticker emoji.
func TestMessage_RecapsAStoredOversizedReplyQuoteText(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("q", domain.ExcerptLength+50)
	addMessages(t, s, domain.Message{
		ID: "reply", ConversationID: "chat", Text: "sure", Created: 1,
		ReplyTo: &domain.Reply{RemoteID: "1", SenderID: "alex", SenderName: "Alex", Text: huge},
	})

	got, err := s.Message(ctx, "reply")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}

	want := domain.Excerpt(huge)
	if got.ReplyTo == nil || got.ReplyTo.Text != want {
		t.Errorf("stored reply quote text = %+v, want Text %q", got.ReplyTo, want)
	}
}
