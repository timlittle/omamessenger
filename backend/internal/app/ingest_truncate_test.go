package app_test

// ingest_truncate_test.go covers Ingest's own cap on a sender-
// controlled message's text, split out of ingest_test.go to keep that
// file within the project's own line-length guideline.

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestIncoming_TruncatesAnOversizedText reproduces a sender-controlled
// message with no length limit at all: Incoming must cap it before
// storing, never store it verbatim, since an unbounded message could
// otherwise make the UI lay out an unbounded amount of rich text in one
// bubble (see ui/lib/Markdown.js and docs/decisions.md).
func TestIncoming_TruncatesAnOversizedText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	long := strings.Repeat("a", domain.MaxIncomingTextLength+1000)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("big", long))

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if len(messages[0].Text) > domain.MaxIncomingTextLength {
		t.Errorf("stored text is %d bytes, want at most %d", len(messages[0].Text), domain.MaxIncomingTextLength)
	}
	if !strings.HasSuffix(messages[0].Text, "[message truncated]") {
		t.Error("stored text does not say it was truncated")
	}
}

// TestEdited_TruncatesAnOversizedText covers the same cap on an edit's
// own new text, which is just as sender-controlled as a newly arrived
// message's.
func TestEdited_TruncatesAnOversizedText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e2", "first"))
	f.published.take()

	long := strings.Repeat("b", domain.MaxIncomingTextLength+1000)
	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{RemoteID: "e2", Text: long})

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want one stored message", messages, err)
	}
	if len(messages[0].Text) > domain.MaxIncomingTextLength {
		t.Errorf("stored text is %d bytes, want at most %d", len(messages[0].Text), domain.MaxIncomingTextLength)
	}
}
