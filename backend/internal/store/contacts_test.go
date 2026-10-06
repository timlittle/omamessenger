package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestUpsertContact_RenamesExisting(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	for _, name := range []string{"Alice Smith", "Alice Cooper"} {
		if err := s.UpsertContact(ctx, domain.Contact{AccountID: "wa", RemoteID: "1", Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Contact(ctx, "wa", "1")
	if err != nil || got.Name != "Alice Cooper" {
		t.Fatalf("Contact = %+v, %v", got, err)
	}

	if _, err := s.Contact(ctx, "wa", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Contact(missing) = %v, want ErrNotFound", err)
	}
}

func TestContacts_SearchIgnoresCaseAndSortsByName(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	for _, c := range []domain.Contact{
		{AccountID: "wa", RemoteID: "3", Name: "Zara Khan"},
		{AccountID: "wa", RemoteID: "2", Name: "alice Jones"},
		{AccountID: "wa", RemoteID: "1", Name: "Alice Cooper"},
	} {
		if err := s.UpsertContact(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"ALICE", []string{"Alice Cooper", "alice Jones"}},
		{"", []string{"Alice Cooper", "alice Jones", "Zara Khan"}},
		{"%", []string{}},
	}

	for _, tt := range tests {
		contacts, err := s.Contacts(ctx, "wa", tt.query)
		if err != nil {
			t.Fatal(err)
		}

		names := []string{}
		for _, c := range contacts {
			names = append(names, c.Name)
		}

		if !slices.Equal(names, tt.want) {
			t.Errorf("Contacts(%q) = %v, want %v", tt.query, names, tt.want)
		}
	}
}
