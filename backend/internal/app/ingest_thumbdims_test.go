package app_test

// ingest_thumbdims_test.go confirms Incoming, History and Edited all
// publish a message's thumbnail with its own verified ThumbWidth and
// ThumbHeight already set, not just the box a sender's claimed photo
// size describes - the gap a security review found let a live,
// backfilled or edited message reach the UI without the size only a
// later read of the row used to compute (see docs/decisions.md).

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fakeThumb returns a plain, solid-colour PNG of the given size,
// base64-encoded exactly as domain.Media.Thumb stores one: a real,
// valid preview a test can plant on a message.
func fakeThumb(t *testing.T, width, height int) string {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode fake thumbnail: %v", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestIncoming_PublishesVerifiedThumbDimensions confirms a live
// message's message.added event already carries its thumbnail's own
// verified ThumbWidth/ThumbHeight, not just the sender's claimed photo
// size: without the fix, only a later read of the row computed these,
// so a message delivered live reached the UI with the thumbnail's box
// alone (see docs/decisions.md).
func TestIncoming_PublishesVerifiedThumbDimensions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	m := incoming("in-1", "a photo")
	m.Media = &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8), Width: 60000, Height: 1}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, m)

	published, ok := f.published.lastOf(app.EventMessageAdded).(domain.Message)
	if !ok || published.Media == nil {
		t.Fatalf("published message.added = %+v, want media attached", published)
	}
	if published.Media.ThumbWidth != 8 || published.Media.ThumbHeight != 8 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 8x8 (the thumbnail's own real size)",
			published.Media.ThumbWidth, published.Media.ThumbHeight)
	}
}

// TestHistory_PublishesVerifiedThumbDimensions is
// TestIncoming_PublishesVerifiedThumbDimensions's own case for a
// backfilled history page: scrolling further back, or the first open
// of a conversation whose history was not yet paged locally, must
// reach the UI with the same verified ThumbWidth/ThumbHeight a live
// message gets, not the thumbnail's box alone.
func TestHistory_PublishesVerifiedThumbDimensions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	m := incoming("h-1", "an older photo")
	m.Status = domain.StatusReceived
	m.Media = &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8), Width: 60000, Height: 1}
	f.ingest.History(ctx, "wa", chat.RemoteID, m)

	published, ok := f.published.lastOf(app.EventMessageAdded).(domain.Message)
	if !ok || published.Media == nil {
		t.Fatalf("published message.added = %+v, want media attached", published)
	}
	if published.Media.ThumbWidth != 8 || published.Media.ThumbHeight != 8 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 8x8 (the thumbnail's own real size)",
			published.Media.ThumbWidth, published.Media.ThumbHeight)
	}
}

// TestEdited_PublishesVerifiedThumbDimensions is
// TestIncoming_PublishesVerifiedThumbDimensions's own case for an
// edit: an edit that attaches or replaces media must reach the UI's
// message.updated event with the same verified ThumbWidth/ThumbHeight
// a live message gets, so a sender cannot re-arm the box-only decode
// bomb by editing an older message (see docs/decisions.md).
func TestEdited_PublishesVerifiedThumbDimensions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("e1", "first"))
	f.published.take()

	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{
		RemoteID: "e1", SenderID: "alex", Text: "now with a photo",
		Media: &domain.Media{Kind: domain.MediaPhoto, Thumb: fakeThumb(t, 8, 8), Width: 60000, Height: 1},
	})

	published, ok := f.published.lastOf(app.EventMessageUpdated).(domain.Message)
	if !ok || published.Media == nil {
		t.Fatalf("published message.updated = %+v, want media attached", published)
	}
	if published.Media.ThumbWidth != 8 || published.Media.ThumbHeight != 8 {
		t.Errorf("ThumbWidth/ThumbHeight = %dx%d, want 8x8 (the thumbnail's own real size)",
			published.Media.ThumbWidth, published.Media.ThumbHeight)
	}
}
