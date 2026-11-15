package app_test

// ingest_dropped_test.go covers the two ways Ingest drops an update
// silently from the store's point of view - a conversation the store
// refuses, and a message whose conversation it has not seen yet - and
// confirms both are logged, never silently, and that a busy or broken
// chat cannot flood the log with the same drop over and over (see
// privacy.md and docs/decisions.md on log flooding).

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConversation_LogsARefusedConversation confirms an invalid
// conversation (missing account, remote id or title) is logged by
// service, never by title or remote id.
func TestConversation_LogsARefusedConversation(t *testing.T) {
	t.Parallel()

	f, ctx := freshFixture(t)
	f.ingest.Conversation(ctx, domain.Conversation{AccountID: "wa", Service: domain.ServiceWhatsApp})

	lines := f.logger.take()
	if len(lines) != 1 || !strings.Contains(lines[0], "conversation") || !strings.Contains(lines[0], domain.ServiceWhatsApp) {
		t.Fatalf("logged lines = %v, want one mentioning the dropped conversation and its service", lines)
	}
	if strings.Contains(lines[0], "r1") {
		t.Errorf("logged line %q named the remote id", lines[0])
	}
}

// TestIncoming_LogsAMessageForAnUnknownConversation confirms a message
// for a conversation Ingest has not seen yet is logged, never by its
// remote id (a phone number for a direct chat on some services).
func TestIncoming_LogsAMessageForAnUnknownConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.ingest.Incoming(t.Context(), "wa", "nowhere", incoming("x", "lost"))

	lines := f.logger.take()
	if len(lines) != 1 || !strings.Contains(lines[0], "message") {
		t.Fatalf("logged lines = %v, want one mentioning the dropped message", lines)
	}
	if strings.Contains(lines[0], "nowhere") {
		t.Errorf("logged line %q named the remote id", lines[0])
	}
}

// TestIncoming_DedupesRepeatedUnknownConversationDrops confirms a flood
// of messages for the same still-unknown conversation logs the drop
// once, not once per message, and logs again once the dedupe window
// has passed.
func TestIncoming_DedupesRepeatedUnknownConversationDrops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)

		for i := 0; i < 5; i++ {
			f.ingest.Incoming(t.Context(), "wa", "nowhere", incoming("x", "lost"))
		}

		if lines := f.logger.take(); len(lines) != 1 {
			t.Fatalf("logged lines after a burst = %v, want exactly one", lines)
		}

		time.Sleep(2 * time.Minute)

		f.ingest.Incoming(t.Context(), "wa", "nowhere", incoming("y", "lost"))
		if lines := f.logger.take(); len(lines) != 1 {
			t.Fatalf("logged lines after the dedupe window passed = %v, want exactly one more", lines)
		}
	})
}
