package app_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConversationRemoteIDChanged_RenamesInPlaceAndPublishes confirms
// Ingest wires a connector's resolved remote id into the store (see
// connector.RemoteIDUpgrader) and republishes the conversation under
// its corrected id, the way a Telegram placeholder's real access hash
// resolving after the fact must correct the stored conversation
// rather than leave a second one.
func TestConversationRemoteIDChanged_RenamesInPlaceAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	placeholder, _, err := f.store.EnsureConversation(ctx, domain.Conversation{
		ID: "ph", AccountID: "wa", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.published.take()

	f.ingest.ConversationRemoteIDChanged(ctx, "wa", "user:42:0", "user:42:67890")

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v, want exactly one conversation update", got)
	}

	updated, err := f.store.Conversation(ctx, placeholder.ID)
	if err != nil || updated.RemoteID != "user:42:67890" {
		t.Errorf("conversation after upgrade = %+v, %v; want remote id %q", updated, err, "user:42:67890")
	}
}

// TestConversationRemoteIDChanged_IgnoresAnUnknownOldID confirms a
// connector's own idempotent check against a peer that never actually
// needed a placeholder - the ordinary case, run on every connect -
// changes and publishes nothing.
func TestConversationRemoteIDChanged_IgnoresAnUnknownOldID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	f.ingest.ConversationRemoteIDChanged(t.Context(), "wa", "user:42:0", "user:42:67890")

	if got := f.published.take(); len(got) != 0 {
		t.Errorf("events = %v, want none", got)
	}
}
