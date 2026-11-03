package whatsapp

import (
	"slices"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMembers_NamesEachParticipant(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("1111", types.DefaultUserServer)
	bob := types.NewJID("2222", types.DefaultUserServer)
	dev, _, c := connectedFixture(t)
	dev.contactNames = map[string]string{alice.String(): "Alice"}
	dev.participants = map[string][]types.GroupParticipant{
		"120363000000000000@g.us": {{JID: alice}, {JID: bob, DisplayName: "Participant"}},
	}

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "120363000000000000@g.us"})
	if err != nil {
		t.Fatal(err)
	}

	want := []domain.Member{
		{ID: remoteID(alice), Name: "Alice", Verified: true},
		{ID: remoteID(bob), Name: "Participant", Verified: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("members = %+v, want %+v", got, want)
	}
}

// TestMembers_MarksABarePushNameUnverified confirms a member resolved
// only through a push name cached in the contact store - no saved name
// or verified business name behind it, the same gap senderDisplayName
// already closes for a message's own sender - reports Verified false:
// a group member's own name is just as able to carry a self-chosen
// claim, such as "Mum", as a direct chat's sender is (see
// docs/decisions.md).
func TestMembers_MarksABarePushNameUnverified(t *testing.T) {
	t.Parallel()

	mallory := types.NewJID("4444", types.DefaultUserServer)
	dev, _, c := connectedFixture(t)
	dev.unverifiedContactNames = map[string]string{mallory.String(): "Mum"}
	dev.participants = map[string][]types.GroupParticipant{"120363000000000002@g.us": {{JID: mallory}}}

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "120363000000000002@g.us"})
	if err != nil {
		t.Fatal(err)
	}

	want := []domain.Member{{ID: remoteID(mallory), Name: "Mum", Verified: false}}
	if !slices.Equal(got, want) {
		t.Errorf("members = %+v, want %+v", got, want)
	}
}

func TestMembers_FallsBackToTheGenericName(t *testing.T) {
	t.Parallel()

	ghost := types.NewJID("3333", types.DefaultUserServer)
	dev, _, c := connectedFixture(t)
	dev.participants = map[string][]types.GroupParticipant{"120363000000000001@g.us": {{JID: ghost}}}

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "120363000000000001@g.us"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != genericSenderName || !got[0].Verified {
		t.Errorf("members = %+v, want the generic name, verified", got)
	}
}

func TestMembers_RejectsAnInvalidRemoteID(t *testing.T) {
	t.Parallel()

	_, _, c := connectedFixture(t)

	if _, err := c.Members(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}); err == nil {
		t.Error("Members: want an error for a bad remote id")
	}
}
