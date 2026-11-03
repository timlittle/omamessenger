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

// TestEnsureConversation_RoundTripsPhone confirms a direct chat's own
// Phone - shown beside an unverified title the way the official
// WhatsApp app shows a stranger's number - is stored and read back
// exactly as given, and stays empty when the service gives none.
func TestEnsureConversation_RoundTripsPhone(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	withPhone, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: "r1", Title: "Mum", Phone: "+447700900000",
	})
	if err != nil || withPhone.Phone != "+447700900000" {
		t.Fatalf("EnsureConversation(with phone) = %+v, %v, want Phone +447700900000", withPhone, err)
	}

	reread, err := s.Conversation(ctx, withPhone.ID)
	if err != nil || reread.Phone != "+447700900000" {
		t.Fatalf("Conversation after creation = %+v, %v, want Phone +447700900000", reread, err)
	}

	noPhone, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "wa", RemoteID: "r2", Title: "Group",
	})
	if err != nil || noPhone.Phone != "" {
		t.Fatalf("EnsureConversation(no phone) = %+v, %v, want Phone empty", noPhone, err)
	}
}
