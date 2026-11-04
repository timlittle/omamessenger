package app_test

// media_displaycopy_test.go covers displayCopy's and tryDisplayCopy's
// own choices once building a photo or sticker's display copy fails
// or needs the StillFrame fallback: when a failure takes the original
// down with it (a genuinely unsafe or undecodable file) versus when
// it does not (an unrelated cache failure), and the cache-identity
// and StillFrame-fallback edge cases around it. Split out of
// media_test.go, and later grown further, to keep that file within
// the project's own line-length guideline.

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestFetchMedia_KeepsTheOriginalWhenTheDisplayCopyFailsForAnUnrelatedReason
// covers a cache failure - a full disk, say - while building a
// photo's display copy: unlike bytes that genuinely fail imagesafe's
// own check (see TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches
// below), this is not the original's own fault, and for an
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

	if _, _, _, _, _, _, err := commands.FetchMedia(ctx, sent.ID, false, false); err == nil {
		t.Fatal("FetchMedia() succeeded despite the cache failure")
	}

	if _, statErr := os.Stat(original); statErr != nil {
		t.Errorf("original attachment stat = %v, want it left alone after an unrelated cache failure", statErr)
	}
}

// TestFetchMedia_FallsBackToStillFrameWhenReencodeCannotDecodeTheOriginal
// covers a sticker (or photo) format Go's own decoders cannot read at
// all - an animated WebP, at least (see docs/decisions.md): displayCopy
// falls back to the sandboxed transcoder's StillFrame profile, then
// Reencode's that extracted frame the same way any other photo is, so
// the display copy it produces is built - and checked - exactly the
// same way regardless of which path got there.
func TestFetchMedia_FallsBackToStillFrameWhenReencodeCannotDecodeTheOriginal(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Sticker]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaSticker, FileName: "sticker.webp"},
	}); err != nil {
		t.Fatal(err)
	}

	// Not a format Go's own decoders can read at all - an animated
	// WebP is the real-world case, but any undecodable bytes exercise
	// the same fallback - so the ordinary Reencode must fail before
	// the fallback even runs.
	f.media.data = []byte("not an image Go can decode at all")

	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil {
		t.Fatalf("FetchMedia() = %v, want the StillFrame fallback to still produce a display copy", err)
	}
	if f.transcoder.lastCall() != "stillframe" {
		t.Errorf("transcoder's last call = %q, want \"stillframe\"", f.transcoder.lastCall())
	}

	view, err := os.ReadFile(path)
	if err != nil || len(view) == 0 {
		t.Errorf("display copy built from the still frame = %v, %v, want real bytes", view, err)
	}
}

// TestFetchMedia_ReportsTheOriginalRefusalWhenTheStillFrameFallbackAlsoFails
// confirms a sticker or photo Reencode refuses, and the sandboxed
// transcoder cannot help with either, still fails the same way any
// other unsafe image already does, rather than crashing or silently
// treating the fallback's own different failure as success.
func TestFetchMedia_ReportsTheOriginalRefusalWhenTheStillFrameFallbackAlsoFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Sticker]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaSticker, FileName: "sticker.webp"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("not an image at all, and the transcoder can't help either")
	f.transcoder.err = errors.New("ffmpeg: no video stream found")

	if _, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false); err == nil {
		t.Fatal("FetchMedia() succeeded despite both Reencode and the StillFrame fallback failing")
	}
}

// TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches reproduces
// the security review's regression finding: a photo cached by an
// older release, before imagesafe covered photos at all, whose bytes
// are not even one of the formats imagesafe recognizes (SVG, here,
// not PNG/JPEG/GIF/WebP). FetchMedia must never hand that file to the
// UI just because it is already on disk under the name a fresh fetch
// would use today: it removes it and downloads a fresh, sanitized
// copy instead.
func TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(f.mediaDir, "m1.photo.jpg")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}

	// SVG is not a video or animation either, so the real sandboxed
	// transcoder's StillFrame fallback would refuse it exactly like
	// Reencode did; the fake must be told to refuse it too, or it
	// would "rescue" the legacy file with its own always-valid still
	// frame instead of exercising the removal-and-redownload path
	// this test is actually about.
	f.transcoder.err = errors.New("ffmpeg: no video stream found")
	f.media.data = testPNGBytes(t, 2, 2)
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil {
		t.Fatalf("FetchMedia() = %v, want the unsafe legacy file replaced by a fresh, safe download", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	assertSamePixels(t, f.media.data, got)
	if !slices.Equal(f.media.fetched, []string{"40"}) {
		t.Errorf("fetched = %v, want exactly one re-download", f.media.fetched)
	}
}

// TestFetchMedia_NeverServesAnUnsafeFileWhenNoSafeReplacementExists
// covers the other half: when even the retry fails the same image
// check, FetchMedia fails safely - a *MediaFetchError, never a path -
// rather than falling back to the unsafe bytes it already rejected
// once.
func TestFetchMedia_NeverServesAnUnsafeFileWhenNoSafeReplacementExists(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(f.mediaDir, "m1.photo.jpg")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if err := os.WriteFile(legacy, svg, 0o600); err != nil {
		t.Fatal(err)
	}

	// See TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches's
	// own comment for why the fake transcoder must refuse SVG too.
	f.transcoder.err = errors.New("ffmpeg: no video stream found")

	// The fake connector's retry returns the same unrecognized bytes,
	// so neither the legacy file nor the retry ever passes.
	f.media.data = svg

	var fetchErr *app.MediaFetchError
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if path != "" || !errors.As(err, &fetchErr) {
		t.Fatalf("FetchMedia() = %q, %v; want a *MediaFetchError and no path", path, err)
	}

	if _, statErr := os.Stat(legacy); !os.IsNotExist(statErr) {
		t.Errorf("legacy file stat = %v, want it removed rather than left trusted", statErr)
	}
}

// TestFetchMedia_RemovesAnUnsafeStickerCacheFileAndRefetches mirrors
// TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches for a
// sticker: cached under whatever extension its sender's claimed file
// name sniffed to (see cache.FetchNamed), never a fixed one, so the
// legacy file sits under "<id>.sticker.svg" here rather than
// "<id>.photo.jpg".
func TestFetchMedia_RemovesAnUnsafeStickerCacheFileAndRefetches(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Sticker]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaSticker},
	}); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(f.mediaDir, "m1.sticker.svg")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}

	f.media.data = testPNGBytes(t, 1, 1)
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil {
		t.Fatalf("FetchMedia() = %v, want the unsafe legacy sticker replaced by a fresh, safe download", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	assertSamePixels(t, f.media.data, got)
}

// TestFetchMedia_FailsFastOnASecondFetchAfterARefusedDisplayCopy
// covers the other half of the "never serves an unsafe file" finding
// above: once a message's display copy has failed its check, the one
// re-download retry included, a second FetchMedia for the very same
// message must not download or decode it all over again - the UI
// auto-fetches every visible photo, so a hostile one would otherwise
// pay that cost again on every scroll (see refusedDisplayCopies and
// docs/decisions.md). It fails fast with the same reason, and the
// fake connector's own FetchMedia is never asked a second time.
func TestFetchMedia_FailsFastOnASecondFetchAfterARefusedDisplayCopy(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	// See TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches's
	// own comment for why the fake transcoder must refuse SVG too.
	f.transcoder.err = errors.New("ffmpeg: no video stream found")
	f.media.data = []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)

	var firstErr *app.MediaFetchError
	if _, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false); !errors.As(err, &firstErr) {
		t.Fatalf("first FetchMedia() = %v, want a *MediaFetchError", err)
	}
	// The first fetch already costs two downloads on its own: the
	// initial attempt, and the one re-download retry
	// recoverUnsafeImage gives a message with nothing cached yet
	// before it gives up (see TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches's
	// own comment on the same retry, there seeded from a cache hit
	// instead). What matters here is that a second, separate
	// FetchMedia call pays nothing further at all.
	firstFetchDownloads := len(f.media.fetched)
	if firstFetchDownloads == 0 {
		t.Fatal("downloads after the first fetch = 0, want at least one real attempt")
	}

	var secondErr *app.MediaFetchError
	if _, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false); !errors.As(err, &secondErr) {
		t.Fatalf("second FetchMedia() = %v, want the same kind of *MediaFetchError, fast", err)
	}
	if secondErr.Reason != firstErr.Reason {
		t.Errorf("second fetch reason = %q, want the same %q the first fetch already found", secondErr.Reason, firstErr.Reason)
	}
	if got := len(f.media.fetched); got != firstFetchDownloads {
		t.Errorf("downloads after the second fetch = %d, want still exactly %d: it must fail fast, not download again", got, firstFetchDownloads)
	}
}

// TestFetchMedia_EditingKindNeverReusesTheOldKindsCachedFile
// reproduces the sharp-edges finding: a "voice note" whose actual
// downloaded bytes are an image is cached unchecked, since a voice
// note carries no image check at all (see app.servedPath). The
// sender then edits the message into a photo - WhatsApp's edit is
// end-to-end encrypted, so any kind change is possible. The edited
// fetch must never reuse the voice note's own cached file: its cache
// identity includes the media kind (see cacheIdentity), so it is a
// clean cache miss, and the fresh attempt - even offered the very
// same unrecognized bytes again - is still caught by the photo's own
// check.
func TestFetchMedia_EditingKindNeverReusesTheOldKindsCachedFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	// See TestFetchMedia_RemovesAnUnsafeLegacyCacheFileAndRefetches's
	// own comment for why the fake transcoder must refuse SVG too.
	f.transcoder.err = errors.New("ffmpeg: no video stream found")
	f.media.data = svg
	voiceMsg := incoming("e1", "[Voice message]")
	voiceMsg.Media = &domain.Media{Kind: domain.MediaVoice, FileName: "voice.ogg"}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, voiceMsg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("Messages() = %+v, %v; want exactly one", messages, err)
	}
	localID := messages[0].ID

	voicePath, _, _, _, _, _, err := f.commands.FetchMedia(ctx, localID, false, false)
	if err != nil {
		t.Fatalf("FetchMedia(voice) = %v", err)
	}

	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{
		RemoteID: "e1", SenderID: "alex", Text: "look", Media: &domain.Media{Kind: domain.MediaPhoto},
	})

	if _, _, _, _, _, _, err := f.commands.FetchMedia(ctx, localID, false, false); err == nil {
		t.Fatal("FetchMedia(edited to photo) succeeded, want the unsafe bytes rejected")
	}

	if _, statErr := os.Stat(voicePath); statErr != nil {
		t.Errorf("the voice note's own cached file was touched: %v, want it left alone", statErr)
	}
}
