package app_test

// ingest_typing_test.go covers Ingest.Typing, split out of
// ingest_test.go to keep that file within this codebase's file-length
// limit.

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestTyping_PublishesForKnownConversations confirms a typing event
// for a known conversation publishes its name and verified flag
// unchanged, and that an unknown conversation publishes nothing. A
// bare, self-chosen name - a group member's own push name, say - must
// publish unverified rather than silently inheriting a previous call's
// true, which would hide exactly the gap a stranger naming themselves
// "Mum" exploits (see docs/decisions.md).
func TestTyping_PublishesForKnownConversations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	f.ingest.Typing(t.Context(), "wa", chat.RemoteID, "Alex", connector.TypingState{Verified: true, Active: true})
	want := app.Typing{ConversationID: chat.ID, Name: "Alex", Verified: true, Active: true}
	if got := f.published.last(); got != want {
		t.Errorf("typing = %+v, want %+v", got, want)
	}

	f.ingest.Typing(t.Context(), "wa", chat.RemoteID, "Mum", connector.TypingState{Verified: false, Active: true})
	want = app.Typing{ConversationID: chat.ID, Name: "Mum", Verified: false, Active: true}
	if got := f.published.last(); got != want {
		t.Errorf("typing = %+v, want %+v", got, want)
	}

	f.ingest.Typing(t.Context(), "wa", "nowhere", "Alex", connector.TypingState{Verified: true, Active: true})
	if got := f.published.take(); !slices.Equal(got, []string{app.EventTyping, app.EventTyping}) {
		t.Errorf("events = %v", got)
	}
}
