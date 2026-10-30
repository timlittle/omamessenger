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
			Width: 100000, Height: 1,
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
	if got.Media.Width != 100000 || got.Media.Height != 1 {
		t.Errorf("Width/Height = %dx%d, want the sender's own claim (100000x1) left untouched", got.Media.Width, got.Media.Height)
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
