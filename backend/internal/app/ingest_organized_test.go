// ingest_organized_test.go covers Ingest.Organized - a dialog sync's
// report of a conversation's pinned and archived state - split into
// its own file rather than added to the already large ingest_test.go,
// to keep that file within the project's own line-length guideline.
package app_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestOrganized_UpdatesAndPublishesOnlyOnAChange confirms Organized
// records a conversation's pinned and archived state and publishes it,
// but only when the sync actually changes something: a sync reporting
// what is already stored, such as the service re-sending the dialog
// list, must not nag the UI with a conversation.updated event that
// changes nothing in it.
func TestOrganized_UpdatesAndPublishesOnlyOnAChange(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	f.ingest.Organized(ctx, "wa", chat.RemoteID, false, false)
	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events after reporting the already-stored state = %v, want none", got)
	}

	f.ingest.Organized(ctx, "wa", chat.RemoteID, true, true)
	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events after an actual change = %v, want one conversation.updated", got)
	}

	stored, err := f.store.Conversation(ctx, chat.ID)
	if err != nil || !stored.Pinned || !stored.Archived {
		t.Fatalf("Conversation after Organized = %+v, %v; want Pinned and Archived true", stored, err)
	}

	// Reporting that same, now-current state again changes nothing
	// further.
	f.ingest.Organized(ctx, "wa", chat.RemoteID, true, true)
	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events after reporting the current state again = %v, want none", got)
	}
}

// TestOrganized_IgnoresAnUnknownConversation confirms a sync naming a
// conversation this account has never seen is silently ignored, rather
// than failing deep inside the store with nothing useful to say: a
// dialog sync tends to run ahead of the conversation list being
// populated.
func TestOrganized_IgnoresAnUnknownConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	f.ingest.Organized(ctx, "wa", "no-such-conversation", true, true)

	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events for an unknown conversation = %v, want none", got)
	}
}
