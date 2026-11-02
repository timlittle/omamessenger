package store_test

// thumbsafe_test.go confirms a message's ThumbWidth and ThumbHeight
// are always the thumbnail bytes' own real, decoder-confirmed size,
// never a sender's separate, unrelated claim about the full photo or
// sticker's own size - the gap a security review found let a lying
// width and height make the UI's own bounded sourceSize cover far
// more than its display box (see docs/decisions.md).

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// TestMessage_ThumbDimensionsAreVerifiedNotClaimed confirms
// ThumbWidth and ThumbHeight, read back through store.sanitizedThumb,
// match the thumbnail's own real, decoded size even when the
// message's Media.Width and Media.Height claim something else
// entirely - the shape a sender lying about a photo's own size takes.
func TestMessage_ThumbDimensionsAreVerifiedNotClaimed(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: "chat", Text: "hi", Created: 1,
		Media: &domain.Media{
			Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8),
			// A sender-claimed full size wildly unlike the thumbnail's
			// own real 8x8 shape: exactly the mismatch the UI must
			// never size a decode from.
			Width: 60000, Height: 1,
		},
	})

	got, err := s.Message(ctx, "m1")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.Media == nil {
		t.Fatal("Media = nil, want it loaded")
	}
	if got.Media.ThumbWidth != 8 || got.Media.ThumbHeight != 8 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 8x8 (the thumbnail's own real size)", got.Media.ThumbWidth, got.Media.ThumbHeight)
	}
	if got.Media.Width != 60000 || got.Media.Height != 1 {
		t.Errorf("Width/Height = %dx%d, want the sender's own claim (60000x1) left untouched", got.Media.Width, got.Media.Height)
	}
}

// TestMessage_ThumbDimensionsAreZeroWithNoThumbnail confirms a
// message with no thumbnail at all reports no verified dimensions
// either, rather than some stale or default value.
func TestMessage_ThumbDimensionsAreZeroWithNoThumbnail(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: "chat", Text: "hi", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "a.txt"},
	})

	got, err := s.Message(ctx, "m1")
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if got.Media == nil {
		t.Fatal("Media = nil, want it loaded")
	}
	if got.Media.ThumbWidth != 0 || got.Media.ThumbHeight != 0 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 0x0 with no thumbnail", got.Media.ThumbWidth, got.Media.ThumbHeight)
	}
}

// TestAddMessage_ReturnsVerifiedThumbDimensions confirms the message
// AddMessage itself returns - the one app.Ingest publishes as
// message.added, with no read of the row back out of the database in
// between - already carries ThumbWidth/ThumbHeight, not just a row
// fetched afterwards with Message or Messages. Before this, a live or
// backfilled message reached the UI with the thumbnail's box alone,
// because only a later read ran sanitizedThumb (see docs/decisions.md).
func TestAddMessage_ReturnsVerifiedThumbDimensions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	got, _, err := s.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: "chat", Text: "hi", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8), Width: 60000, Height: 1},
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if got.Media == nil || got.Media.ThumbWidth != 8 || got.Media.ThumbHeight != 8 {
		t.Fatalf("AddMessage's own return: ThumbWidth/ThumbHeight = %+v, want 8x8", got.Media)
	}
}

// TestEditMessage_ReturnsVerifiedThumbDimensions is
// TestAddMessage_ReturnsVerifiedThumbDimensions's own case for an edit:
// EditMessage's return value - what app.Ingest publishes as
// message.updated - must carry the edited media's own verified
// ThumbWidth/ThumbHeight too, not only a later read of the row.
func TestEditMessage_ReturnsVerifiedThumbDimensions(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1})

	got, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{
		Text:  "hi, with a photo",
		Media: &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8), Width: 60000, Height: 1},
	})
	if err != nil || !found {
		t.Fatalf("EditMessage: found=%v, err=%v", found, err)
	}
	if got.Media == nil || got.Media.ThumbWidth != 8 || got.Media.ThumbHeight != 8 {
		t.Fatalf("EditMessage's own return: ThumbWidth/ThumbHeight = %+v, want 8x8", got.Media)
	}
}
