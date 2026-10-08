package store_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

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

// TestConversations_SearchReportsTheMatchedMessageIDAndSender checks that
// a message match names which message and who sent it, not just its text:
// the unified command palette's "Messages" section needs both to show a
// sender and to open and highlight that exact message.
func TestConversations_SearchReportsTheMatchedMessageIDAndSender(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "chat", SenderName: "Alex", Text: "ticket opened", Created: 1},
		domain.Message{ID: "m2", ConversationID: "chat", SenderName: "Priya", Text: "ticket closed", Created: 2},
	)

	got, err := s.Conversations(t.Context(), "ticket")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 || got[0].MatchMessageID != "m2" || got[0].MatchSender != "Priya" {
		t.Fatalf("Conversations(ticket)[0] = %+v, want MatchMessageID m2 and MatchSender Priya", got[0])
	}

	// A title-only match, or no match at all, names no message.
	titleOnly, err := s.Conversations(t.Context(), "Chat")
	if err != nil {
		t.Fatal(err)
	}
	if titleOnly[0].MatchMessageID != "" || titleOnly[0].MatchSender != "" {
		t.Errorf("title-only match = %+v, want no matched message", titleOnly[0])
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

func TestConversations_SearchFollowsEditsAndDeletes(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "lunch on friday", Created: 1})

	if _, found, err := s.EditMessage(ctx, "chat", "1", store.MessageEdit{Text: "dinner on saturday"}); err != nil || !found {
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
