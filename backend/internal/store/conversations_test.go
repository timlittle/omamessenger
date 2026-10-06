package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestEnsureConversation_CreatesThenFollowsRemote(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	first := domain.Conversation{ID: "chat", AccountID: "wa", RemoteID: "remote", Title: "Old", Members: 2}
	created, isNew, err := s.EnsureConversation(ctx, first)
	if err != nil || !isNew || created.Kind != domain.KindDirect || created.Service != domain.ServiceWhatsApp {
		t.Fatalf("first EnsureConversation = %+v, %t, %v", created, isNew, err)
	}

	second := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "New", Kind: domain.KindGroup, Members: 12}
	updated, isNew, err := s.EnsureConversation(ctx, second)
	if err != nil || isNew || updated.ID != "chat" || updated.Title != "New" ||
		updated.Kind != domain.KindGroup || updated.Members != 12 {
		t.Fatalf("second EnsureConversation = %+v, %t, %v", updated, isNew, err)
	}
}

func TestEnsureConversation_RejectsMissingFields(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	for _, c := range []domain.Conversation{
		{RemoteID: "remote", Title: "Title"},
		{AccountID: "wa", Title: "Title"},
		{AccountID: "wa", RemoteID: "remote"},
	} {
		if _, _, err := s.EnsureConversation(t.Context(), c); !errors.Is(err, store.ErrInvalidConversation) {
			t.Errorf("EnsureConversation(%+v) = %v, want ErrInvalidConversation", c, err)
		}
	}
}

func TestConversation_NotFound(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	if _, err := s.Conversation(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Conversation(missing) = %v, want ErrNotFound", err)
	}

	if _, err := s.ConversationByRemote(t.Context(), "wa", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConversationByRemote(missing) = %v, want ErrNotFound", err)
	}
}

func TestConversations_SearchesTitlesAndMessages(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "title-hit", "Release_100% Notes")
	addConversation(t, s, "wa", "message-hit", "A chat")
	addConversation(t, s, "wa", "other", "Other")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "message-hit", Text: "ticket 50% pending", Created: 10},
		domain.Message{ID: "m2", ConversationID: "message-hit", Text: "newer ticket 50% resolved", Created: 20},
		domain.Message{ID: "m3", ConversationID: "other", Text: "underscore _ literal", Created: 30},
	)

	tests := []struct {
		query     string
		want      []string
		wantMatch string
	}{
		{"", []string{"other", "message-hit", "title-hit"}, ""},
		{"ticket", []string{"message-hit"}, "newer ticket 50% resolved"},
		{"100%", []string{"title-hit"}, ""},
		{"_", []string{"other", "title-hit"}, "underscore _ literal"},
		{"  RELEASE_100% NOTES  ", []string{"title-hit"}, ""},
	}

	for _, tt := range tests {
		got, err := s.Conversations(t.Context(), tt.query)
		if err != nil {
			t.Fatal(err)
		}

		var gotIDs []string
		for _, c := range got {
			gotIDs = append(gotIDs, c.ID)
		}

		if !slices.Equal(gotIDs, tt.want) || got[0].Match != tt.wantMatch {
			t.Errorf("Conversations(%q) = %v match %q, want %v match %q",
				tt.query, gotIDs, got[0].Match, tt.want, tt.wantMatch)
		}
	}
}

func TestMarkRead_ReportsChange(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	if changed, err := s.MarkRead(ctx, "chat"); err != nil || !changed {
		t.Fatalf("first MarkRead = %t, %v; want true", changed, err)
	}

	if changed, err := s.MarkRead(ctx, "chat"); err != nil || changed {
		t.Fatalf("second MarkRead = %t, %v; want false", changed, err)
	}

	if _, err := s.MarkRead(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkRead(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetUnread_TakesTheServicesCount(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	if changed, err := s.SetUnread(ctx, "chat", 3); err != nil || !changed {
		t.Fatalf("SetUnread(3) = %t, %v; want a change", changed, err)
	}

	if c, _ := s.Conversation(ctx, "chat"); c.Unread != 3 {
		t.Errorf("unread = %d, want 3", c.Unread)
	}

	if changed, err := s.SetUnread(ctx, "chat", 3); err != nil || changed {
		t.Errorf("SetUnread(3) again = %t, %v; want no change", changed, err)
	}

	if _, err := s.SetUnread(ctx, "missing", 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetUnread(missing) = %v, want ErrNotFound", err)
	}
}

func TestUnreadTotal_IgnoresMutedConversations(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	for _, step := range []struct {
		muted bool
		want  int
	}{{true, 0}, {false, 1}} {
		if err := s.SetMuted(ctx, "chat", step.muted); err != nil {
			t.Fatal(err)
		}

		if total, err := s.UnreadTotal(ctx); err != nil || total != step.want {
			t.Errorf("UnreadTotal with muted=%t = %d, %v; want %d", step.muted, total, err, step.want)
		}
	}

	if err := s.SetMuted(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetMuted(missing) = %v, want ErrNotFound", err)
	}
}
