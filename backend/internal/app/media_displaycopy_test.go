package app_test

// media_displaycopy_test.go covers tryDisplayCopy's own choice of
// when a failed display copy takes its original down with it - split
// out of media_test.go to keep that file within the project's own
// line-length guideline. The "removed" side of that choice is already
// exercised in media_test.go, where bytes that genuinely fail
// imagesafe's check also happen to be a cache's legacy-file scenario;
// this file covers the other side, a failure that says nothing about
// the original's own bytes.

import (
	"errors"
	"os"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestFetchMedia_KeepsTheOriginalWhenTheDisplayCopyFailsForAnUnrelatedReason
// covers a cache failure - a full disk, say - while building a
// photo's display copy: unlike bytes that genuinely fail imagesafe's
// own check (see TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches
// in media_test.go), this is not the original's own fault, and for an
// outgoing message that original is the attachment a still-pending
// send needs to upload. It must be left exactly where it was, never
// removed just because building the copy failed for some unrelated
// reason.
func TestFetchMedia_KeepsTheOriginalWhenTheDisplayCopyFailsForAnUnrelatedReason(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 2, 2)

	sent, err := f.commands.Send(ctx, "chat", "look", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	original := f.outgoing.Path(sent.ID, "photo.png")
	if _, statErr := os.Stat(original); statErr != nil {
		t.Fatalf("attachment missing before the failing fetch: %v", statErr)
	}

	// A fresh Commands over the same store and outgoing directory, but
	// with a cache that fails every Fetch before fill (the re-encode)
	// ever runs - the shape of a full disk, never a verdict on the
	// attachment's own bytes.
	commands, _ := app.New(app.Deps{
		Store: f.store, Media: f.media, Outgoing: f.outgoing,
		Cache: &fakeCache{err: errors.New("cache: disk full")}, Logger: f.logger,
	})

	if _, _, _, err := commands.FetchMedia(ctx, sent.ID, false); err == nil {
		t.Fatal("FetchMedia() succeeded despite the cache failure")
	}

	if _, statErr := os.Stat(original); statErr != nil {
		t.Errorf("original attachment stat = %v, want it left alone after an unrelated cache failure", statErr)
	}
}
