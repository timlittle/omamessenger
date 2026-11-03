package store_test

// messages_oversizedid_test.go confirms a message whose own RemoteID
// or SenderID runs past domain.MaxRemoteIDLength never reaches the UI
// on a read, even though it was stored directly here rather than
// through app.Ingest, which now refuses to store one shaped this way:
// a row a release before that refusal existed already wrote must
// still be caught every time it is read, the same way scanMessage
// already repairs every other field (see messages_sanitize_test.go).
// Unlike those fields, RemoteID and SenderID are never truncated or
// cleared - they are the dedup key and the handle React, Edit and
// Delete key off - so a row shaped like this is skipped outright:
// Message reports it as not found, and Messages drops it from the
// page, keeping every other row.

import (
	"errors"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMessage_TreatsAStoredOversizedRemoteIDAsNotFound confirms a
// single-message read reports domain.ErrNotFound for a row whose own
// RemoteID runs past the cap, the same as a row that is not there at
// all.
func TestMessage_TreatsAStoredOversizedRemoteIDAsNotFound(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("1", domain.MaxRemoteIDLength+1)
	addMessages(t, s, domain.Message{ID: "huge", ConversationID: "chat", RemoteID: huge, Text: "hi", Created: 1})

	if _, err := s.Message(ctx, "huge"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Message error = %v, want %v", err, domain.ErrNotFound)
	}
}

// TestMessage_TreatsAStoredOversizedSenderIDAsNotFound mirrors
// TestMessage_TreatsAStoredOversizedRemoteIDAsNotFound for SenderID.
func TestMessage_TreatsAStoredOversizedSenderIDAsNotFound(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"
	addMessages(t, s, domain.Message{ID: "huge", ConversationID: "chat", RemoteID: "r1", SenderID: huge, Text: "hi", Created: 1})

	if _, err := s.Message(ctx, "huge"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Message error = %v, want %v", err, domain.ErrNotFound)
	}
}

// TestMessageByRemote_TreatsAStoredOversizedRemoteIDAsNotFound
// confirms the same refusal for the lookup EditMessage, SetReactions
// and SetPoll all use to find the message they update: a legacy
// oversized row must not be returned for any of them to republish.
func TestMessageByRemote_TreatsAStoredOversizedRemoteIDAsNotFound(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("1", domain.MaxRemoteIDLength+1)
	addMessages(t, s, domain.Message{ID: "huge", ConversationID: "chat", RemoteID: huge, Text: "hi", Created: 1})

	if _, err := s.MessageByRemote(ctx, "chat", huge); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MessageByRemote error = %v, want %v", err, domain.ErrNotFound)
	}
}

// TestMessages_DropsAStoredOversizedRowFromThePage confirms a page
// listing skips a legacy oversized row outright, rather than failing
// the whole page or returning the row as-is, keeping every other
// message.
func TestMessages_DropsAStoredOversizedRowFromThePage(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("1", domain.MaxRemoteIDLength+1)
	addMessages(t, s,
		domain.Message{ID: "ok", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1},
		domain.Message{ID: "huge", ConversationID: "chat", RemoteID: huge, Text: "hi", Created: 2},
	)

	got, _, err := s.Messages(ctx, "chat", "", 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok" {
		t.Errorf("Messages = %+v, want only the message with a real-sized id", got)
	}
}
