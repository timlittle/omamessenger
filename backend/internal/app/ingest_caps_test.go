package app_test

// ingest_caps_test.go covers Ingest's own caps on a sender-controlled
// message's formatting spans, mentions, reactions and poll or link
// preview media, split out of ingest_truncate_test.go to keep each
// file focused on one kind of cap.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestIncoming_CapsFormattingSpans reproduces a sender-controlled
// formatting entity list with no limit of its own: Incoming must cap
// it before storing, never store it verbatim.
func TestIncoming_CapsFormattingSpans(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming("spans", "hello")
	for i := 0; i < domain.MaxFormattingSpans+20; i++ {
		msg.Formatting = append(msg.Formatting, domain.Span{Kind: domain.SpanBold, Start: 0, End: 1})
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if got := len(messages[0].Formatting); got != domain.MaxFormattingSpans {
		t.Errorf("stored %d formatting spans, want exactly %d", got, domain.MaxFormattingSpans)
	}
}

// TestIncoming_CapsMentions mirrors TestIncoming_CapsFormattingSpans
// for a message's mentions.
func TestIncoming_CapsMentions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming("mentions", "hello")
	for i := 0; i < domain.MaxMentions+20; i++ {
		msg.Mentions = append(msg.Mentions, domain.Mention{UserID: "u", Name: "x", Offset: 0, Length: 1})
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if got := len(messages[0].Mentions); got != domain.MaxMentions {
		t.Errorf("stored %d mentions, want exactly %d", got, domain.MaxMentions)
	}
}

// TestIncoming_CapsReactions mirrors TestIncoming_CapsFormattingSpans
// for a message's reaction chips.
func TestIncoming_CapsReactions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming("reactions", "hello")
	for i := 0; i < domain.MaxReactions+20; i++ {
		msg.Reactions = append(msg.Reactions, domain.Reaction{Emoji: "x", Count: 1})
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if got := len(messages[0].Reactions); got != domain.MaxReactions {
		t.Errorf("stored %d reactions, want exactly %d", got, domain.MaxReactions)
	}
}

// TestIncoming_CapsPollOptions covers a poll whose service-reported
// option count has no limit of its own.
func TestIncoming_CapsPollOptions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	options := make([]domain.PollOption, domain.MaxPollOptions+20)
	for i := range options {
		options[i] = domain.PollOption{ID: "o", Text: "option"}
	}
	msg := incoming("poll", "[Poll]")
	msg.Media = &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{Question: "q?", Options: options}}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Media == nil || messages[0].Media.Poll == nil {
		t.Fatalf("messages = %+v, %v; want one stored poll message", messages, err)
	}
	if got := len(messages[0].Media.Poll.Options); got != domain.MaxPollOptions {
		t.Errorf("stored %d poll options, want exactly %d", got, domain.MaxPollOptions)
	}
}

// TestEdited_CapsFormattingSpans confirms an edit's new formatting
// spans get the same cap a newly arrived message's do: both go through
// the one shared domain.SanitizeIncoming call.
func TestEdited_CapsFormattingSpans(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "first"))
	f.published.take()

	edit := domain.Message{RemoteID: "e1", Text: "second"}
	for i := 0; i < domain.MaxFormattingSpans+20; i++ {
		edit.Formatting = append(edit.Formatting, domain.Span{Kind: domain.SpanBold, Start: 0, End: 1})
	}
	f.ingest.Edited(ctx, "wa", chat.RemoteID, edit)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if got := len(messages[0].Formatting); got != domain.MaxFormattingSpans {
		t.Errorf("stored %d formatting spans after edit, want exactly %d", got, domain.MaxFormattingSpans)
	}
}
