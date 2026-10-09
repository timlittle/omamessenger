package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestDeleteMessages_FallsBackPreviewAndLowersUnread(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", SenderName: "Alex", Text: "first", Created: 1},
		domain.Message{ID: "m2", ConversationID: "chat", RemoteID: "2", SenderName: "Alex", Text: "second", Created: 2},
	)

	deleted, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, []string{"2"})
	if err != nil || !slices.Equal(ids(deleted), []string{"m2"}) {
		t.Fatalf("DeleteMessages = %v, %v", ids(deleted), err)
	}

	chat, err := s.Conversation(ctx, "chat")
	if err != nil || chat.Preview != "first" || chat.LastActivity != 1 || chat.Unread != 1 {
		t.Errorf("conversation after deletion = %+v, %v", chat, err)
	}

	if _, err := s.Message(ctx, "m2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleted message still stored: %v", err)
	}
}

// TestDeleteMessages_OnlyTouchesConversationsInScope reproduces a bug: a
// private chat and a channel can both hold a message under the same
// remote id, because Telegram numbers channel messages in their own
// space, so a deletion naming only the chat must never reach the
// channel's copy. The caller, not the store, decides which conversations
// an id might belong to.
func TestDeleteMessages_OnlyTouchesConversationsInScope(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Private Chat")
	addConversation(t, s, "wa", "channel", "Channel")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "5", Text: "a", Created: 1},
		domain.Message{ID: "m2", ConversationID: "channel", RemoteID: "5", Text: "b", Created: 1},
	)

	deleted, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, []string{"5"})
	if err != nil || !slices.Equal(ids(deleted), []string{"m1"}) {
		t.Fatalf("DeleteMessages(chat only) = %v, %v", ids(deleted), err)
	}

	if _, err := s.Message(ctx, "m2"); err != nil {
		t.Errorf("message outside the scope was touched: %v", err)
	}
}

func TestDeleteMessages_IgnoresUnknownRemoteIDs(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "a", Created: 1})

	deleted, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, []string{"missing"})
	if err != nil || len(deleted) != 0 {
		t.Fatalf("DeleteMessages(unknown remote id) = %v, %v; want none deleted", deleted, err)
	}
}

func TestDeleteMessages_IgnoresAnEmptyScope(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "a", Created: 1})

	deleted, err := s.DeleteMessages(ctx, "wa", nil, []string{"1"})
	if err != nil || len(deleted) != 0 {
		t.Fatalf("DeleteMessages(no scope) = %v, %v; want none deleted", deleted, err)
	}
}

func TestDeleteMessages_IgnoresAnEmptyRemoteIDList(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "a", Created: 1})

	deleted, err := s.DeleteMessages(ctx, "wa", []string{"r-chat"}, nil)
	if err != nil || deleted != nil {
		t.Fatalf("DeleteMessages(no remote ids) = %v, %v; want nil, nil", deleted, err)
	}
}

// TestDeleteMessages_SkipsAConversationRemoteIDThatIsNotFound confirms a
// deletion naming several conversations still deletes a message from
// whichever of them are found, skipping the rest, rather than failing
// the whole call because one named conversation does not exist: a
// connector resolving conversationRemoteIDs from its own chat list can
// legitimately name one this store never saw.
func TestDeleteMessages_SkipsAConversationRemoteIDThatIsNotFound(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "1", Text: "a", Created: 1})

	deleted, err := s.DeleteMessages(ctx, "wa", []string{"r-missing", "r-chat"}, []string{"1"})
	if err != nil || !slices.Equal(ids(deleted), []string{"m1"}) {
		t.Fatalf("DeleteMessages(unknown conversation first) = %v, %v", ids(deleted), err)
	}
}
