package store_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestAddMessage_UpdatesPreviewAndUnread(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	incoming, _, err := s.AddMessage(ctx, domain.Message{
		ID: "in", ConversationID: "chat", RemoteID: "r1", Text: "hello", SenderName: "Alex", Created: 200,
	})
	if err != nil || incoming.Status != domain.StatusReceived {
		t.Fatalf("incoming = %+v, %v", incoming, err)
	}

	outgoing, _, err := s.AddMessage(ctx, domain.Message{
		ID: "out", ConversationID: "chat", RemoteID: "r2", Text: "reply", Outgoing: true, Created: 300,
	})
	if err != nil || outgoing.Status != domain.StatusPending {
		t.Fatalf("outgoing = %+v, %v", outgoing, err)
	}

	// Older history must not replace the preview.
	addMessages(t, s, domain.Message{ID: "old", ConversationID: "chat", Text: "older", SenderName: "Earlier", Created: 100})

	chat, err := s.Conversation(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}

	if chat.Unread != 2 || chat.Preview != "reply" || !chat.PreviewOut || chat.PreviewSender != "" || chat.LastActivity != 300 {
		t.Errorf("conversation = %+v", chat)
	}
}

func TestAddMessage_IgnoresDuplicateRemoteID(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "first", ConversationID: "chat", RemoteID: "r1", Text: "hello", Created: 1})

	dup, inserted, err := s.AddMessage(ctx, domain.Message{ConversationID: "chat", RemoteID: "r1", Text: "again", Created: 2})
	if err != nil || inserted || dup.ID != "first" {
		t.Fatalf("duplicate AddMessage = %+v, %t, %v", dup, inserted, err)
	}

	chat, _ := s.Conversation(ctx, "chat")
	if chat.Unread != 1 || chat.Preview != "hello" {
		t.Errorf("duplicate changed the conversation: %+v", chat)
	}
}

func TestAddMessage_RejectsInvalidMessages(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()

	for _, m := range []domain.Message{{Text: "text"}, {ConversationID: "chat"}} {
		if _, _, err := s.AddMessage(ctx, m); !errors.Is(err, store.ErrInvalidMessage) {
			t.Errorf("AddMessage(%+v) = %v, want ErrInvalidMessage", m, err)
		}
	}

	if _, _, err := s.AddMessage(ctx, domain.Message{ConversationID: "missing", Text: "text"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("AddMessage(unknown conversation) = %v, want ErrNotFound", err)
	}
}

func TestMessages_KeepsArrivalOrderWithinAMillisecond(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s,
		domain.Message{ID: "a", ConversationID: "chat", Text: "a", Created: 999},
		domain.Message{ID: "b", ConversationID: "chat", Text: "b", Created: 1000},
		domain.Message{ID: "c", ConversationID: "chat", Text: "c", Created: 2000},
		domain.Message{ID: "d", ConversationID: "chat", Text: "d", Created: 2000},
	)

	page, _, err := s.Messages(t.Context(), "chat", "", 50)
	if err != nil || !slices.Equal(ids(page), []string{"a", "b", "c", "d"}) {
		t.Fatalf("Messages = %v, %v", ids(page), err)
	}
}

func TestMessages_PagesBackwards(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	for i := 1; i <= 120; i++ {
		addMessages(t, s, domain.Message{ID: fmt.Sprintf("m%03d", i), ConversationID: "chat", Text: "x", Created: int64(i)})
	}

	before := ""
	for _, want := range []struct {
		first, last string
		count       int
		more        bool
	}{
		{"m071", "m120", 50, true},
		{"m021", "m070", 50, true},
		{"m001", "m020", 20, false},
	} {
		page, more, err := s.Messages(ctx, "chat", before, 50)
		if err != nil || len(page) != want.count || more != want.more ||
			page[0].ID != want.first || page[len(page)-1].ID != want.last {
			t.Fatalf("Messages(before %q) = %v, more %t, %v", before, ids(page), more, err)
		}

		before = page[0].ID
	}
}

func TestOldestRemoteID_SkipsMessagesTheServiceHasNotNumbered(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if id, err := s.OldestRemoteID(ctx, "chat"); err != nil || id != "" {
		t.Errorf("OldestRemoteID(empty chat) = %q, %v; want none", id, err)
	}

	addMessages(t, s,
		domain.Message{ID: "draft", ConversationID: "chat", Text: "unsent", Created: 1},
		domain.Message{ID: "m2", ConversationID: "chat", RemoteID: "20", Text: "b", Created: 2},
		domain.Message{ID: "m3", ConversationID: "chat", RemoteID: "30", Text: "c", Created: 3},
	)

	if id, err := s.OldestRemoteID(ctx, "chat"); err != nil || id != "20" {
		t.Errorf("OldestRemoteID = %q, %v; want 20", id, err)
	}
}

func TestMessages_LimitsAndCursor(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	for i := range store.DefaultPageSize + 1 {
		addMessages(t, s, domain.Message{ConversationID: "chat", Text: "x", Created: int64(i)})
	}

	for _, limit := range []int{0, -1, store.MaxPageSize + 1} {
		page, _, err := s.Messages(ctx, "chat", "", limit)
		if err != nil || len(page) != store.DefaultPageSize {
			t.Errorf("Messages(limit %d) = %d messages, %v; want the default page", limit, len(page), err)
		}
	}

	if _, _, err := s.Messages(ctx, "chat", "unknown", 10); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Messages(unknown cursor) = %v, want ErrNotFound", err)
	}

	if page, _, err := s.Messages(ctx, "missing", "", 10); err != nil || len(page) != 0 {
		t.Errorf("Messages(unknown conversation) = %v, %v; want empty", page, err)
	}
}

func TestUpdateMessageStatus_OnlyMovesForward(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "out", ConversationID: "chat", Text: "hi", Outgoing: true, Created: 1})

	for _, step := range []struct {
		to      string
		changed bool
		want    string
	}{
		{domain.StatusSent, true, domain.StatusSent},
		{domain.StatusRead, true, domain.StatusRead},
		{domain.StatusDelivered, false, domain.StatusRead},
	} {
		got, changed, err := s.UpdateMessageStatus(ctx, "out", step.to)
		if err != nil || changed != step.changed || got.Status != step.want {
			t.Fatalf("UpdateMessageStatus(%s) = %s, %t, %v", step.to, got.Status, changed, err)
		}
	}

	if _, _, err := s.UpdateMessageStatus(ctx, "missing", domain.StatusSent); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateMessageStatus(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetMessageRemoteID_FindsByRemote(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "out", ConversationID: "chat", Text: "hi", Outgoing: true, Created: 1})

	if err := s.SetMessageRemoteID(ctx, "out", "remote-1"); err != nil {
		t.Fatal(err)
	}

	got, err := s.MessageByRemote(ctx, "chat", "remote-1")
	if err != nil || got.ID != "out" {
		t.Fatalf("MessageByRemote = %+v, %v", got, err)
	}

	if _, err := s.MessageByRemote(ctx, "chat", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MessageByRemote(empty id) = %v, want ErrNotFound", err)
	}
}

func TestAddMessage_KeepsMediaAndFillsItInLater(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	link := &domain.Media{Kind: domain.MediaLink, URL: "https://x.io", SiteName: "X", Title: "A page", Description: "About it"}

	addMessages(t, s,
		domain.Message{ID: "with", ConversationID: "chat", RemoteID: "1", Text: "see https://x.io", Created: 1, Media: link},
		domain.Message{ID: "without", ConversationID: "chat", RemoteID: "2", Text: "https://y.io", Created: 2},
	)

	got, err := s.Message(ctx, "with")
	if err != nil || got.Media == nil || *got.Media != *link {
		t.Errorf("stored media = %+v, %v; want %+v", got.Media, err, link)
	}

	// The service reports the second message again, now with its preview.
	again, inserted, err := s.AddMessage(ctx, domain.Message{ConversationID: "chat", RemoteID: "2", Text: "https://y.io", Created: 2, Media: link})
	if err != nil || inserted || again.ID != "without" || again.Media == nil {
		t.Fatalf("AddMessage again = %+v, inserted %t, %v; want the stored message with its media", again, inserted, err)
	}

	if got, _ := s.Message(ctx, "without"); got.Media == nil || got.Media.Title != "A page" {
		t.Errorf("media not filled in: %+v", got.Media)
	}
}
