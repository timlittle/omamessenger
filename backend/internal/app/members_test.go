package app_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMembers_ListsAGroupsMembers(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	group := f.conversation(t, "group", "Group", domain.KindGroup)
	f.members.members = []domain.Member{{ID: "u1", Name: "Nadia"}}

	got, err := f.commands.Members(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, f.members.members) {
		t.Errorf("Members = %+v, want %+v", got, f.members.members)
	}
	if !slices.Equal(f.members.asked, []string{group.ID}) {
		t.Errorf("asked = %v, want [%s]", f.members.asked, group.ID)
	}
}

func TestMembers_EmptyForADirectChat(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.members.members = []domain.Member{{ID: "u1", Name: "Nadia"}}

	got, err := f.commands.Members(t.Context(), chat.ID)
	if err != nil || len(got) != 0 {
		t.Errorf("Members(direct) = %+v, %v, want an empty list", got, err)
	}
	if len(f.members.asked) != 0 {
		t.Errorf("asked = %v, want no connector call for a direct chat", f.members.asked)
	}
}

func TestMembers_UnknownConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.Members(t.Context(), "missing"); err == nil {
		t.Error("Members(missing): want an error")
	}
}
