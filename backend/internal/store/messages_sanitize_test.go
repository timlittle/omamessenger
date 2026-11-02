package store_test

// messages_sanitize_test.go confirms a message's own sender-controlled
// text - its text, its sender name, a reply's quoted sender name, and
// its reaction chips - is capped on every read, even when the row was
// stored uncapped by a release before domain.SanitizeIncoming's caps
// existed, or by any other path that skips it. See store.scanMessage.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMessage_CapsAStoredSenderNameAndReplySenderName confirms a
// message's own SenderName and a reply's quoted SenderName, stored
// uncapped by a release before domain.MaxDisplayNameLength existed
// (or by any other path that skips domain.SanitizeIncoming), are capped
// on every read, the same defence scanMessage already gives a legacy
// thumbnail (see sanitizedThumb) and a legacy sticker emoji.
func TestMessage_CapsAStoredSenderNameAndReplySenderName(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("x", domain.MaxDisplayNameLength+1)
	addMessages(t, s, domain.Message{
		ID: "huge", ConversationID: "chat", Text: "hi", Created: 1, SenderName: huge,
		ReplyTo: &domain.Reply{RemoteID: "not-loaded", SenderName: huge, Text: "quoted"},
	})

	got, err := s.Message(ctx, "huge")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if domain.UTF16Len(got.SenderName) != domain.MaxDisplayNameLength {
		t.Errorf("SenderName length = %d, want %d", domain.UTF16Len(got.SenderName), domain.MaxDisplayNameLength)
	}
	if got.ReplyTo == nil || domain.UTF16Len(got.ReplyTo.SenderName) != domain.MaxDisplayNameLength {
		t.Errorf("ReplyTo.SenderName = %+v, want capped to %d", got.ReplyTo, domain.MaxDisplayNameLength)
	}
}

// TestMessage_CapsStoredReactionsOnRead confirms reaction chips stored
// uncapped by a release before domain.MaxReactions and
// domain.MaxReactionEmojiLength existed are capped on every read: an
// oversized chip's own emoji text is dropped outright, and the chip
// count never exceeds domain.MaxReactions.
func TestMessage_CapsStoredReactionsOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	reactions := make([]domain.Reaction, 0, domain.MaxReactions+2)
	for i := 0; i < domain.MaxReactions+1; i++ {
		reactions = append(reactions, domain.Reaction{Emoji: fmt.Sprintf("%d", i%10), Count: 1})
	}
	reactions = append(reactions, domain.Reaction{Emoji: strings.Repeat("x", domain.MaxReactionEmojiLength+1), Count: 1})

	addMessages(t, s, domain.Message{ID: "reacted", ConversationID: "chat", Text: "hi", Created: 1, Reactions: reactions})

	got, err := s.Message(ctx, "reacted")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if len(got.Reactions) != domain.MaxReactions {
		t.Errorf("len(Reactions) = %d, want %d", len(got.Reactions), domain.MaxReactions)
	}
	for _, r := range got.Reactions {
		if domain.UTF16Len(r.Emoji) > domain.MaxReactionEmojiLength {
			t.Errorf("kept an oversized reaction emoji: %q", r.Emoji)
		}
	}
}

// TestMessage_CapsAStoredOversizedMediaURLOnRead confirms a link
// preview's own URL, stored uncapped by a release before
// domain.MaxURLLength existed, is dropped on every read, the same
// legacy-row repair scanMessage already gives a stored sender name or
// reaction chip: a truncated URL would point somewhere the sender
// never wrote, so it must be dropped outright, never shortened.
func TestMessage_CapsAStoredOversizedMediaURLOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := "https://example.com/" + strings.Repeat("a", domain.MaxURLLength)
	addMessages(t, s, domain.Message{
		ID: "huge-url", ConversationID: "chat", Text: "hi", Created: 1,
		Media: &domain.Media{Kind: domain.MediaLink, URL: huge, Title: "Example"},
	})

	got, err := s.Message(ctx, "huge-url")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.Media == nil || got.Media.URL != "" {
		t.Errorf("Media.URL = %+v, want it dropped", got.Media)
	}
}

// TestMessage_CapsAStoredOversizedMentionUserIDOnRead confirms a
// mention's own UserID, stored uncapped by a release before
// domain.MaxRemoteIDLength existed, is cleared on every read, the same
// legacy-row repair scanMessage already gives a stored reaction chip
// or link preview URL: a truncated id could read as a different,
// shorter real one it was never actually sent as, so it must be
// cleared outright, never shortened.
func TestMessage_CapsAStoredOversizedMentionUserIDOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"
	addMessages(t, s, domain.Message{
		ID: "huge-mention", ConversationID: "chat", Text: "@x", Created: 1,
		Mentions: []domain.Mention{{UserID: huge, Name: "x", Offset: 0, Length: 2}},
	})

	got, err := s.Message(ctx, "huge-mention")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if len(got.Mentions) != 1 {
		t.Fatalf("Mentions = %+v, want the one mention kept", got.Mentions)
	}
	if got.Mentions[0].UserID != "" {
		t.Errorf("Mentions[0].UserID = %q, want it dropped", got.Mentions[0].UserID)
	}
}

// TestMessage_CapsAStoredOversizedReplyRemoteIDOnRead confirms a reply
// quote's own RemoteID, stored uncapped by a release before
// domain.MaxRemoteIDLength existed, is cleared on every read, the same
// legacy-row repair scanMessage already gives a stored reply SenderName.
func TestMessage_CapsAStoredOversizedReplyRemoteIDOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("r", domain.MaxRemoteIDLength+1)
	addMessages(t, s, domain.Message{
		ID: "huge-reply", ConversationID: "chat", Text: "hi", Created: 1,
		ReplyTo: &domain.Reply{RemoteID: huge, SenderName: "Quoted", Text: "quoted"},
	})

	got, err := s.Message(ctx, "huge-reply")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.ReplyTo == nil || got.ReplyTo.RemoteID != "" {
		t.Errorf("ReplyTo.RemoteID = %+v, want it dropped", got.ReplyTo)
	}
	if got.ReplyTo.SenderName != "Quoted" {
		t.Errorf("ReplyTo.SenderName = %q, want it untouched", got.ReplyTo.SenderName)
	}
}

// TestMessage_TruncatesStoredTextOnRead confirms a message's own text,
// stored longer than domain.MaxIncomingTextLength by a release before
// that cap existed, is truncated on every read: the one unbounded
// field a hostile sender's own history could otherwise still make the
// UI shape megabytes of text for, every time the message scrolls into
// view.
func TestMessage_TruncatesStoredTextOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("a", domain.MaxIncomingTextLength+1000)
	addMessages(t, s, domain.Message{ID: "huge", ConversationID: "chat", Text: huge, Created: 1})

	got, err := s.Message(ctx, "huge")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if len(got.Text) > domain.MaxIncomingTextLength {
		t.Errorf("len(Text) = %d, want at most %d", len(got.Text), domain.MaxIncomingTextLength)
	}
}
