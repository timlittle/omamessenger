package store_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestEnsureConversation_RoundTripsTitleVerified confirms a
// conversation's TitleVerified is stored and read back exactly as
// given, both on creation and on a later refresh, the same way Title
// itself already round-trips: the UI's "~" marking depends on this
// surviving a restart, not just the first report (see
// docs/decisions.md).
func TestEnsureConversation_RoundTripsTitleVerified(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	created, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: "remote", Title: "Mum", TitleVerified: false,
	})
	if err != nil || created.TitleVerified {
		t.Fatalf("EnsureConversation(unverified) = %+v, %v, want TitleVerified false", created, err)
	}

	reread, err := s.Conversation(ctx, created.ID)
	if err != nil || reread.TitleVerified {
		t.Fatalf("Conversation after creation = %+v, %v, want TitleVerified false", reread, err)
	}

	refreshed, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: "remote", Title: "Mum", TitleVerified: true,
	})
	if err != nil || !refreshed.TitleVerified {
		t.Fatalf("EnsureConversation(verified refresh) = %+v, %v, want TitleVerified true", refreshed, err)
	}

	rereadAgain, err := s.Conversation(ctx, created.ID)
	if err != nil || !rereadAgain.TitleVerified {
		t.Fatalf("Conversation after refresh = %+v, %v, want TitleVerified true", rereadAgain, err)
	}
}
