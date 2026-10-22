package app_test

// ingest_caps_test.go covers Ingest's own caps on a sender-controlled
// message's formatting spans, mentions, reactions and poll or link
// preview media, and on a sender-controlled display name (a contact's
// name, a conversation's title, a typing indicator's name or a
// corrected sender name), split out of ingest_truncate_test.go to keep
// each file focused on one kind of cap.

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
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

// TestReacted_CapsReactions confirms a live reaction update gets the
// same cap on its tally's own size and each chip's emoji text that a
// newly arrived message's own Reactions already does, so a connector's
// own reaction cache cannot be used to grow a conversation's chip list
// past what the UI is ever meant to show.
func TestReacted_CapsReactions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "hi"))
	f.published.take()

	reactions := make([]domain.Reaction, domain.MaxReactions+10)
	for i := range reactions {
		reactions[i] = domain.Reaction{Emoji: "x", Count: 1}
	}
	f.ingest.Reacted(ctx, "wa", chat.RemoteID, "e1", reactions)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if got := len(messages[0].Reactions); got != domain.MaxReactions {
		t.Errorf("stored %d reactions, want exactly %d", got, domain.MaxReactions)
	}
}

// TestContact_CapsName confirms a contact's sender-controlled name is
// capped before it is stored, the same cap SanitizeIncoming gives a
// message's own SenderName.
func TestContact_CapsName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	long := strings.Repeat("n", domain.MaxDisplayNameLength+10)
	f.ingest.Contact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: long})

	c, err := f.store.Contact(ctx, "wa", "c1")
	if err != nil {
		t.Fatalf("Contact = %v", err)
	}
	if n := domain.UTF16Len(c.Name); n != domain.MaxDisplayNameLength {
		t.Errorf("stored contact name at %d units, want exactly %d", n, domain.MaxDisplayNameLength)
	}
}

// TestConversation_CapsTitle confirms a conversation's sender-
// controlled title (a group's own name) is capped before it is stored.
func TestConversation_CapsTitle(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	long := strings.Repeat("t", domain.MaxDisplayNameLength+10)
	f.ingest.Conversation(ctx, domain.Conversation{AccountID: "wa", RemoteID: "r1", Title: long, Kind: domain.KindGroup})

	conv, err := f.store.ConversationByRemote(ctx, "wa", "r1")
	if err != nil {
		t.Fatalf("ConversationByRemote = %v", err)
	}
	if n := domain.UTF16Len(conv.Title); n != domain.MaxDisplayNameLength {
		t.Errorf("stored conversation title at %d units, want exactly %d", n, domain.MaxDisplayNameLength)
	}
}

// TestTyping_CapsName confirms a typing indicator's sender-controlled
// name is capped before it is published.
func TestTyping_CapsName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	long := strings.Repeat("n", domain.MaxDisplayNameLength+10)
	f.ingest.Typing(t.Context(), "wa", chat.RemoteID, long, true)

	got := f.published.last().(app.Typing)
	if n := domain.UTF16Len(got.Name); n != domain.MaxDisplayNameLength {
		t.Errorf("published typing name at %d units, want exactly %d", n, domain.MaxDisplayNameLength)
	}
}

// TestSenderName_CapsTheCorrectedName confirms a sender-name
// correction is capped before it reaches the store, the same cap
// SanitizeIncoming gives a message's own SenderName.
func TestSenderName_CapsTheCorrectedName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	group := f.conversation(t, "group", "Weekend plans", domain.KindGroup)
	f.ingest.Incoming(ctx, "wa", group.RemoteID, incoming("in-1", "Thanks"))

	long := strings.Repeat("n", domain.MaxDisplayNameLength+10)
	f.ingest.SenderName(ctx, "wa", "alex", long)

	updated, err := f.store.Conversation(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := domain.UTF16Len(updated.PreviewSender); n != domain.MaxDisplayNameLength {
		t.Errorf("preview sender at %d units, want exactly %d", n, domain.MaxDisplayNameLength)
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
