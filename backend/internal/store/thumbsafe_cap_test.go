package store_test

// thumbsafe_cap_test.go confirms sanitizedThumb enforces
// domain.MaxInlineThumbLen on its own re-encoded output, not only on
// the raw bytes domain.CapMedia already checked before a thumbnail
// ever reaches the store: a security review found a highly
// compressible PNG well under the cap could still re-encode, as the
// JPEG sanitizedThumb always writes for an opaque image, to several
// hundred kilobytes (see fakeOversizedThumb). A later read already
// caught this by chance - scanMessage runs domain.SanitizeIncoming,
// whose own CapMedia drops an oversized Thumb, after decodeMessageColumns
// has already re-encoded it - but AddMessage, fillMedia and
// EditMessage all return the message they just wrote directly, with
// no such second pass: that returned value, not a later read, is what
// app.Ingest publishes live as message.added or message.updated (see
// docs/decisions.md).

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// bloatedMedia is a photo message's media carrying a thumbnail that
// passes every raw-bytes cap but reencodes, as sanitizedThumb's own
// JPEG output, past domain.MaxInlineThumbLen.
func bloatedMedia(t *testing.T) *domain.Media {
	t.Helper()

	return &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeOversizedThumb(t, 512, 512)}
}

// TestAddMessage_DropsAThumbThatReencodesOverTheCap confirms the
// message AddMessage returns - what app.Ingest publishes live, with
// no read of the row back out in between - never carries a thumbnail
// over the cap, even though the bloat only exists once imagesafe has
// already re-encoded it.
func TestAddMessage_DropsAThumbThatReencodesOverTheCap(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	got, inserted, err := s.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: "chat", Text: "hi", Created: 1, Media: bloatedMedia(t),
	})
	if err != nil || !inserted {
		t.Fatalf("AddMessage: inserted=%v, err=%v", inserted, err)
	}
	if got.Media == nil || got.Media.Thumb != "" {
		t.Errorf("Media = %+v, want the thumbnail dropped, not kept at its oversized, re-encoded length", got.Media)
	}
	if got.Media.ThumbWidth != 0 || got.Media.ThumbHeight != 0 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 0x0 for a dropped thumbnail", got.Media.ThumbWidth, got.Media.ThumbHeight)
	}
}

// TestAddMessage_FillMediaDropsAThumbThatReencodesOverTheCap mirrors
// TestAddMessage_DropsAThumbThatReencodesOverTheCap for fillMedia: a
// second report of the same remote id, from the same sender, adding
// media to a message stored without any yet.
func TestAddMessage_FillMediaDropsAThumbThatReencodesOverTheCap(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if _, inserted, err := s.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r1", SenderID: "alex", Text: "hi", Created: 1,
	}); err != nil || !inserted {
		t.Fatalf("AddMessage (text only): inserted=%v, err=%v", inserted, err)
	}

	got, inserted, err := s.AddMessage(ctx, domain.Message{
		ID: "m2", ConversationID: "chat", RemoteID: "r1", SenderID: "alex", Text: "hi", Created: 1, Media: bloatedMedia(t),
	})
	if err != nil || inserted {
		t.Fatalf("AddMessage (filling media): inserted=%v, err=%v, want a merge into the existing row", inserted, err)
	}
	if got.Media == nil || got.Media.Thumb != "" {
		t.Errorf("Media = %+v, want the thumbnail dropped, not kept at its oversized, re-encoded length", got.Media)
	}
}

// TestEditMessage_DropsAThumbThatReencodesOverTheCap mirrors
// TestAddMessage_DropsAThumbThatReencodesOverTheCap for the edit write
// path: an edit's own new thumbnail is just as able to reencode past
// the cap as a newly arrived message's.
func TestEditMessage_DropsAThumbThatReencodesOverTheCap(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	if _, inserted, err := s.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1,
	}); err != nil || !inserted {
		t.Fatalf("AddMessage: inserted=%v, err=%v", inserted, err)
	}

	updated, found, err := s.EditMessage(ctx, "chat", "r1", store.MessageEdit{Text: "hi (edited)", Media: bloatedMedia(t)})
	if err != nil || !found {
		t.Fatalf("EditMessage: found=%v, err=%v", found, err)
	}
	if updated.Media == nil || updated.Media.Thumb != "" {
		t.Errorf("Media = %+v, want the thumbnail dropped, not kept at its oversized, re-encoded length", updated.Media)
	}
}
