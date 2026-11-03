package app_test

// ingest_oversizedid_test.go covers Ingest refusing to store a message
// whose own RemoteID or SenderID runs past domain.MaxRemoteIDLength,
// for every path that can introduce one: a live message, a backfilled
// one and an edit. Kept apart from ingest_caps_test.go, which covers
// fields SanitizeIncoming repairs in place - RemoteID and SenderID are
// never truncated or cleared, only refused outright (see
// domain.ValidMessageIDs).

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestIncoming_RefusesAnOversizedRemoteID confirms a live message whose
// RemoteID runs past the cap is dropped: never stored, never
// published, and logged only as a safe category.
func TestIncoming_RefusesAnOversizedRemoteID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming(strings.Repeat("1", domain.MaxRemoteIDLength+1), "hello")
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages = %+v, %v; want none stored", messages, err)
	}
	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events = %v, want none published", got)
	}
	if lines := f.logger.take(); len(lines) != 1 || !strings.Contains(lines[0], "oversized") {
		t.Errorf("logged lines = %v, want one mentioning the oversized id", lines)
	}
}

// TestIncoming_RefusesAnOversizedSenderID mirrors
// TestIncoming_RefusesAnOversizedRemoteID for SenderID.
func TestIncoming_RefusesAnOversizedSenderID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming("r1", "hello")
	msg.SenderID = strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages = %+v, %v; want none stored", messages, err)
	}
}

// TestHistory_RefusesAnOversizedRemoteID confirms a backfilled message
// - the shape an undecryptable placeholder or a history sync entry
// takes - gets the same refusal a live message does: both go through
// Ingest's one shared save.
func TestHistory_RefusesAnOversizedRemoteID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	msg := incoming(strings.Repeat("1", domain.MaxRemoteIDLength+1), "hello")
	f.ingest.History(ctx, "wa", chat.RemoteID, msg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages = %+v, %v; want none stored", messages, err)
	}
}

// TestEdited_RefusesAnOversizedRemoteID confirms an edit reporting an
// oversized RemoteID or SenderID is dropped, the same as a newly
// arriving message: an edit is just as able to introduce one as a live
// message, since both carry whatever the connector just read off the
// wire.
func TestEdited_RefusesAnOversizedRemoteID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "first"))
	f.published.take()

	edit := domain.Message{RemoteID: strings.Repeat("1", domain.MaxRemoteIDLength+1), SenderID: "alex", Text: "second"}
	f.ingest.Edited(ctx, "wa", chat.RemoteID, edit)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Text != "first" {
		t.Fatalf("messages = %+v, %v; want the original message untouched", messages, err)
	}
	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events = %v, want none published", got)
	}
}
