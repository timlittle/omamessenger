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
		{ID: remoteID(alice), Name: "Alice"},
		{ID: remoteID(bob), Name: "Participant"},
	}
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
	if len(got) != 1 || got[0].Name != genericSenderName {
		t.Errorf("members = %+v, want the generic name", got)
	}
}

func TestMembers_RejectsAnInvalidRemoteID(t *testing.T) {
	t.Parallel()

	_, _, c := connectedFixture(t)

	if _, err := c.Members(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}); err == nil {
		t.Error("Members: want an error for a bad remote id")
	}
}
