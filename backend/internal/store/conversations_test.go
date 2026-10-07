package store_test

import (
	"errors"
	"path/filepath"
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
		// "_" has no word characters, so it cannot be an FTS5 term: it only
		// matches literally in a title, the way LIKE always has.
		{"_", []string{"title-hit"}, ""},
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

func TestConversations_MatchesWordPrefixes(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", Text: "a ticket arrived", Created: 1})

	for _, query := range []string{"tick", "ticket", "TICK"} {
		got, err := s.Conversations(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 1 || got[0].ID != "chat" {
			t.Errorf("Conversations(%q) = %v, want [chat]", query, got)
		}
	}

	if got, err := s.Conversations(t.Context(), "ticke z"); err != nil || len(got) != 0 {
		t.Errorf("Conversations(%q) = %v, %v, want no match for an unmatched extra word", "ticke z", got, err)
	}
}

func TestConversations_MatchesCaseAndAccentInsensitively(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", Text: "let's meet at the café", Created: 1})

	for _, query := range []string{"cafe", "CAFE", "café", "Café"} {
		got, err := s.Conversations(t.Context(), query)
		if err != nil || len(got) != 1 || got[0].ID != "chat" {
			t.Errorf("Conversations(%q) = %v, %v, want [chat]", query, got, err)
		}
	}
}

func TestConversations_RanksBestOrMostRecentMatchFirst(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "old", "Old chat")
	addConversation(t, s, "wa", "new", "New chat")
	// Both conversations have the same last_activity order as their ids
	// suggest, but "new" has the more recent matching message, so a plain
	// search must rank it first even though title order alone would not.
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "old", Text: "ticket closed", Created: 100},
		domain.Message{ID: "m2", ConversationID: "new", Text: "ticket opened", Created: 1},
		domain.Message{ID: "m3", ConversationID: "new", Text: "ticket updated again", Created: 200},
	)

	got, err := s.Conversations(t.Context(), "ticket")
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID)
	}

	if want := []string{"new", "old"}; !slices.Equal(ids, want) {
		t.Errorf("Conversations(ticket) order = %v, want %v", ids, want)
	}

	if got[0].Match != "ticket updated again" {
		t.Errorf("match = %q, want the newest matching message", got[0].Match)
	}
}

func TestConversations_IgnoresHostileQuerySyntax(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", Text: "a ticket about AND OR NOT", Created: 1})

	queries := []string{
		`"`, `*`, `-`, `AND`, `OR`, `NOT`, `NEAR`, `(`, `)`, `:`,
		`"ticket`, `ticket"`, `ticket*`, `ticket AND foo`, `-ticket`,
		`"" OR 1=1`, `conversation_id:1`,
	}
	for _, query := range queries {
		if _, err := s.Conversations(t.Context(), query); err != nil {
			t.Errorf("Conversations(%q) = %v, want no error", query, err)
		}
	}

	if got, err := s.Conversations(t.Context(), "AND"); err != nil || len(got) != 1 || got[0].ID != "chat" {
		t.Errorf(`Conversations("AND") = %v, %v, want [chat] (literal word, not the FTS5 operator)`, got, err)
	}
}

// FuzzConversations checks that no query text, however it abuses FTS5 or
// SQL LIKE syntax, can make Conversations error or panic: a search box
// must accept anything the user types.
func FuzzConversations(f *testing.F) {
	for _, seed := range []string{
		"", "_", "%", `"`, "*", "-", "AND", "OR", "NOT", "NEAR", "(", ")", ":",
		`"ticket`, `ticket"`, `ticket*`, `""`, `café`, "a\x00b", "a\nb",
	} {
		f.Add(seed)
	}

	path := filepath.Join(f.TempDir(), "data", "messages.db")
	s, err := store.Open(f.Context(), path)
	if err != nil {
		f.Fatalf("Open: %v", err)
	}
	f.Cleanup(func() { _ = s.Close() })

	account := domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "wa"}
	if err := s.UpsertAccount(f.Context(), account); err != nil {
		f.Fatalf("UpsertAccount: %v", err)
	}

	f.Fuzz(func(t *testing.T, query string) {
		if _, err := s.Conversations(t.Context(), query); err != nil {
			t.Errorf("Conversations(%q) = %v, want no error", query, err)
		}
	})
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

func TestSetPinned_PinsAndUnpins(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if err := s.SetPinned(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Pinned {
		t.Fatalf("Conversation after pin = %+v, %v; want Pinned true", c, err)
	}

	if err := s.SetPinned(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Pinned {
		t.Fatalf("Conversation after unpin = %+v, %v; want Pinned false", c, err)
	}

	if err := s.SetPinned(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetPinned(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetArchived_ArchivesAndUnarchives(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if err := s.SetArchived(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Archived {
		t.Fatalf("Conversation after archive = %+v, %v; want Archived true", c, err)
	}

	if err := s.SetArchived(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Archived {
		t.Fatalf("Conversation after unarchive = %+v, %v; want Archived false", c, err)
	}

	if err := s.SetArchived(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetArchived(missing) = %v, want ErrNotFound", err)
	}
}

func TestConversations_OrdersPinnedFirst(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "old-pinned", "Old Pinned")
	addConversation(t, s, "wa", "new", "New")
	addConversation(t, s, "wa", "new-pinned", "New Pinned")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "old-pinned", Text: "one", Created: 10},
		domain.Message{ID: "m2", ConversationID: "new", Text: "two", Created: 20},
		domain.Message{ID: "m3", ConversationID: "new-pinned", Text: "three", Created: 30},
	)

	if err := s.SetPinned(ctx, "old-pinned", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPinned(ctx, "new-pinned", true); err != nil {
		t.Fatal(err)
	}

	// "old-pinned" has the oldest activity of the three but still leads
	// "new", which is not pinned, because pinned conversations always come
	// first; within pinned or unpinned, the newest activity leads.
	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}

	want := []string{"new-pinned", "old-pinned", "new"}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("Conversations order = %v, want %v", gotIDs, want)
	}
}

func TestEnsureConversation_UpdatesPinnedAndArchived(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	first := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "Chat"}
	created, _, err := s.EnsureConversation(ctx, first)
	if err != nil || created.Pinned || created.Archived {
		t.Fatalf("first EnsureConversation = %+v, %v; want neither pinned nor archived", created, err)
	}

	second := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "Chat", Pinned: true, Archived: true}
	updated, _, err := s.EnsureConversation(ctx, second)
	if err != nil || !updated.Pinned || !updated.Archived {
		t.Fatalf("second EnsureConversation = %+v, %v; want pinned and archived", updated, err)
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

func TestConversations_SearchFollowsEditsAndDeletes(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "lunch on friday", Created: 1})

	if _, found, err := s.EditMessage(ctx, "chat", "1", "dinner on saturday", nil); err != nil || !found {
		t.Fatalf("EditMessage = %t, %v", found, err)
	}

	for query, want := range map[string]int{"lunch": 0, "dinner": 1} {
		if got, err := s.Conversations(ctx, query); err != nil || len(got) != want {
			t.Errorf("after the edit, search %q found %d chats (%v), want %d", query, len(got), err, want)
		}
	}

	if _, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, []string{"1"}); err != nil {
		t.Fatal(err)
	}

	if got, err := s.Conversations(ctx, "dinner"); err != nil || len(got) != 0 {
		t.Errorf("after the delete, search found %d chats (%v), want none", len(got), err)
	}
}
