package store_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestAddMessage_UpdatesPreviewAndUnread(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

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

// TestAddMessage_CapsTheStoredPreview confirms bumpConversation caps
// the preview it stores to domain.MaxPreviewLength: a message's own
// Text can be far longer (up to domain.MaxIncomingTextLength), but the
// conversation row never needs more than the one line the UI shows.
func TestAddMessage_CapsTheStoredPreview(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("a", domain.MaxPreviewLength+1000)
	addMessages(t, s, domain.Message{ID: "huge", ConversationID: "chat", Text: huge, Created: 1})

	chat, err := s.Conversation(ctx, "chat")
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if n := domain.UTF16Len(chat.Preview); n != domain.MaxPreviewLength {
		t.Errorf("Preview length = %d, want %d", n, domain.MaxPreviewLength)
	}
}

func TestAddHistoryMessage_NeverCountsTowardsUnread(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	// Status received, same as a connector reports a message it does not
	// know was ever read: AddMessage would count this one, AddHistoryMessage
	// must not, since loading history is backfill, not something new.
	older, _, err := s.AddHistoryMessage(ctx, domain.Message{
		ID: "h1", ConversationID: "chat", RemoteID: "r1", Text: "hello", SenderName: "Alex", Created: 100,
	})
	if err != nil || older.Status != domain.StatusReceived {
		t.Fatalf("older = %+v, %v", older, err)
	}

	chat, err := s.Conversation(ctx, "chat")
	if err != nil || chat.Unread != 0 || chat.Preview != "hello" || chat.LastActivity != 100 {
		t.Fatalf("conversation after AddHistoryMessage = %+v, %v; want unread 0 but the preview still set", chat, err)
	}

	// A newer history message still becomes the preview: only the unread
	// count is special-cased, not the rest of bumpConversation's work.
	if _, _, err := s.AddHistoryMessage(ctx, domain.Message{
		ID: "h2", ConversationID: "chat", RemoteID: "r2", Text: "later", Created: 200,
	}); err != nil {
		t.Fatal(err)
	}

	chat, err = s.Conversation(ctx, "chat")
	if err != nil || chat.Unread != 0 || chat.Preview != "later" || chat.LastActivity != 200 {
		t.Errorf("conversation after a newer AddHistoryMessage = %+v, %v", chat, err)
	}
}

func TestAddMessage_IgnoresDuplicateRemoteID(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
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

	s, _ := openChatStore(t)
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

	s, _ := openChatStore(t)
	ctx := t.Context()

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

	s, _ := openChatStore(t)
	ctx := t.Context()

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

	s, _ := openChatStore(t)
	ctx := t.Context()

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

	s, _ := openChatStore(t)
	ctx := t.Context()
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

// TestUpdateMessageStatus_ConcurrentCallsAdvanceOnlyOnce reproduces two
// callers racing to move the same failed message back to pending - the
// automatic retry scheduler's own pass and a user's manual retry,
// started at the same moment. Both read "failed" and both decide
// "pending" is forward progress, but the update must still apply for
// only one of them: a second send for the same message is a bug, not
// just a wasted store write.
func TestUpdateMessageStatus_ConcurrentCallsAdvanceOnlyOnce(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "out", ConversationID: "chat", Text: "hi", Outgoing: true, Status: domain.StatusFailed, Created: 1})

	const racers = 20
	var changedCount atomic.Int32
	start := make(chan struct{})

	var wg sync.WaitGroup
	for range racers {
		wg.Go(func() {
			<-start
			_, changed, err := s.UpdateMessageStatus(ctx, "out", domain.StatusPending)
			if err != nil {
				t.Errorf("UpdateMessageStatus = %v", err)
			}
			if changed {
				changedCount.Add(1)
			}
		})
	}

	close(start)
	wg.Wait()

	if n := changedCount.Load(); n != 1 {
		t.Errorf("callers that advanced failed to pending = %d, want exactly 1", n)
	}

	if got, err := s.Message(ctx, "out"); err != nil || got.Status != domain.StatusPending {
		t.Errorf("stored status after the race = %+v, %v, want pending", got, err)
	}
}

func TestSetMessageRemoteID_FindsByRemote(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
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

func TestEditMessage_UpdatesTextMediaAndSetsEdited(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	link := &domain.Media{Kind: domain.MediaLink, URL: "https://x.io"}
	got, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{Text: "hi there", Media: link})
	if err != nil || !found || got.Text != "hi there" || got.Media == nil || !got.Edited {
		t.Fatalf("EditMessage = %+v, found %t, %v", got, found, err)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || stored.Text != "hi there" || !stored.Edited || stored.Media == nil {
		t.Errorf("stored message not updated: %+v, %v", stored, err)
	}
}

func TestEditMessage_IgnoresAMessageThatIsNotStored(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	got, found, err := s.EditMessage(ctx, "chat", "missing", store.MessageEdit{Text: "edited"})
	if err != nil || found || got.ID != "" {
		t.Fatalf("EditMessage(missing) = %+v, found %t, %v; want ignored", got, found, err)
	}
}

func TestAddMessage_FillsReplySenderAndTextFromTheQuotedMessage(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "quoted", ConversationID: "chat", RemoteID: "1", SenderID: "r-alex", Text: "original message", SenderName: "Alex", Created: 1})

	reply, _, err := s.AddMessage(ctx, domain.Message{
		ID: "reply", ConversationID: "chat", RemoteID: "2", Text: "sure", Created: 2,
		ReplyTo: &domain.Reply{RemoteID: "1"},
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.SenderName != "Alex" || reply.ReplyTo.Text != "original message" {
		t.Fatalf("reply.ReplyTo = %+v, want sender Alex and the quoted text", reply.ReplyTo)
	}
	if reply.ReplyTo.SenderID != "r-alex" {
		t.Errorf("reply.ReplyTo.SenderID = %q, want the quoted message's own sender id, r-alex", reply.ReplyTo.SenderID)
	}

	stored, err := s.Message(ctx, "reply")
	if err != nil || stored.ReplyTo == nil || stored.ReplyTo.RemoteID != "1" || stored.ReplyTo.SenderName != "Alex" || stored.ReplyTo.SenderID != "r-alex" {
		t.Errorf("stored reply = %+v, %v", stored.ReplyTo, err)
	}
}

func TestAddMessage_KeepsAReplysOwnSenderAndTextWhenAlreadyGiven(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	// The quoted message is not stored at all here: an outgoing reply
	// already carries what it needs, filled in by the app from the
	// message it answers, so the store must not need to look it up.
	reply, _, err := s.AddMessage(ctx, domain.Message{
		ID: "reply", ConversationID: "chat", RemoteID: "2", Text: "sure", Created: 2,
		ReplyTo: &domain.Reply{RemoteID: "1", SenderID: "alex", SenderName: "You", Text: "hi there"},
	})
	if err != nil || reply.ReplyTo == nil || reply.ReplyTo.SenderID != "alex" || reply.ReplyTo.SenderName != "You" || reply.ReplyTo.Text != "hi there" {
		t.Fatalf("AddMessage = %+v, %v", reply.ReplyTo, err)
	}
}

func TestAddMessage_LeavesAReplyToAnUnloadedMessageAsGiven(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	reply, _, err := s.AddMessage(ctx, domain.Message{
		ID: "reply", ConversationID: "chat", RemoteID: "2", Text: "sure", Created: 2,
		ReplyTo: &domain.Reply{RemoteID: "not-loaded"},
	})
	if err != nil || reply.ReplyTo == nil || reply.ReplyTo.RemoteID != "not-loaded" || reply.ReplyTo.SenderName != "" {
		t.Fatalf("AddMessage = %+v, %v, want the bare reply unchanged", reply.ReplyTo, err)
	}
}

func TestAddMessage_KeepsMediaAndFillsItInLater(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
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

// TestAddMessage_RoundTripsAnAttachmentsOriginalFile confirms a media's
// original file path and modification time survive being stored and
// read back, alongside the media itself.
func TestAddMessage_RoundTripsAnAttachmentsOriginalFile(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	media := &domain.Media{Kind: domain.MediaPhoto, FileName: "photo.png", Size: 10, OriginalPath: "/home/user/photo.png", OriginalModTime: 12345}
	addMessages(t, s, domain.Message{
		ID: "out", ConversationID: "chat", Text: "hi", Outgoing: true, Created: 1, Media: media,
	})

	got, err := s.Message(ctx, "out")
	if err != nil || got.Media == nil || got.Media.OriginalPath != media.OriginalPath || got.Media.OriginalModTime != media.OriginalModTime {
		t.Fatalf("stored media = %+v, %v; want the original path and modtime kept", got.Media, err)
	}
}

// TestMessage_SanitizesStoredThumbnail confirms a message's thumbnail
// is checked against imagesafe's own bound every time it is read back,
// not only once by the migration that repairs what a previous version
// already wrote to disk (see docs/decisions.md): a thumbnail whose
// declared size is far over the bound, or that is not valid base64 at
// all, is cleared to "", while an ordinary one is returned unchanged.
// This is the defence that still catches a thumbnail a future write
// path forgets to sanitize before storing it, exactly the way AddMessage
// here never itself checks what it is handed.
func TestMessage_SanitizesStoredThumbnail(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	clean := fakeThumb(t, 8, 8)
	oversized := fakeThumb(t, 2000, 10)

	addMessages(t, s,
		domain.Message{ID: "clean", ConversationID: "chat", Text: "a", Created: 1, Media: &domain.Media{Kind: domain.MediaSticker, Thumb: clean}},
		domain.Message{ID: "oversized", ConversationID: "chat", Text: "b", Created: 2, Media: &domain.Media{Kind: domain.MediaSticker, Thumb: oversized}},
		domain.Message{ID: "invalid", ConversationID: "chat", Text: "c", Created: 3, Media: &domain.Media{Kind: domain.MediaSticker, Thumb: "not valid base64 at all!!"}},
	)

	if got, err := s.Message(ctx, "clean"); err != nil || got.Media.Thumb != clean {
		t.Errorf("clean thumbnail = %q, %v; want %q, unchanged", got.Media.Thumb, err, clean)
	}
	if got, err := s.Message(ctx, "oversized"); err != nil || got.Media.Thumb != "" {
		t.Errorf("oversized thumbnail = %q, %v; want cleared", got.Media.Thumb, err)
	}
	if got, err := s.Message(ctx, "invalid"); err != nil || got.Media.Thumb != "" {
		t.Errorf("invalid-base64 thumbnail = %q, %v; want cleared", got.Media.Thumb, err)
	}

	// Messages, the paging the UI's timeline calls, must give the exact
	// same guarantee: this is the read path a review named directly.
	page, _, err := s.Messages(ctx, "chat", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range page {
		if m.ID == "oversized" && m.Media.Thumb != "" {
			t.Errorf("Messages kept an oversized thumbnail for %q: %q", m.ID, m.Media.Thumb)
		}
		if m.ID == "invalid" && m.Media.Thumb != "" {
			t.Errorf("Messages kept an invalid-base64 thumbnail for %q: %q", m.ID, m.Media.Thumb)
		}
	}
}

// TestMessage_CapsAStoredStickerEmoji confirms a sticker's own Emoji
// fallback, stored uncapped by a release before MaxStickerEmojiLength
// existed, is capped on every read, the same defence scanMessage
// already gives a legacy thumbnail (see sanitizedThumb).
func TestMessage_CapsAStoredStickerEmoji(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	huge := strings.Repeat("x", domain.MaxStickerEmojiLength+1)
	addMessages(t, s,
		domain.Message{ID: "short", ConversationID: "chat", Text: "a", Created: 1, Media: &domain.Media{Kind: domain.MediaSticker, Emoji: "🎉"}},
		domain.Message{ID: "huge", ConversationID: "chat", Text: "b", Created: 2, Media: &domain.Media{Kind: domain.MediaSticker, Emoji: huge}},
	)

	if got, err := s.Message(ctx, "short"); err != nil || got.Media.Emoji != "🎉" {
		t.Errorf("short emoji = %q, %v; want %q, unchanged", got.Media.Emoji, err, "🎉")
	}
	if got, err := s.Message(ctx, "huge"); err != nil || got.Media.Emoji != "" {
		t.Errorf("oversized emoji = %q, %v; want dropped", got.Media.Emoji, err)
	}
}
