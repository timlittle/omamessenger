package app_test

// media_decode_test.go covers the helper's own verdict on how a
// downloaded video or voice note's actual bytes may be decoded -
// DecodeAsForMedia, VideoPlayableForMedia and AudioPlayableForMedia -
// split out of media_test.go to keep that file within the project's
// own line-length guideline.

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/audiosafe/audiosafetest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestDecodeAsForMedia_NeverTrustsAFileNameOrAttribute reproduces a
// Telegram "GIF" that bypassed the image sanitizer: a video document
// carrying the video and animated attributes, and a sender-chosen
// ".gif" name, whose actual downloaded bytes are not GIF data at all.
// DecodeAsForMedia must say "" - never "gif" - whatever the message's
// own metadata claims, since only the file's own bytes decide.
func TestDecodeAsForMedia_NeverTrustsAFileNameOrAttribute(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[GIF]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, FileName: "evil.gif", Animated: true},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("a plain silent mp4, whatever the filename and attributes claim")
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatal(err)
	}

	decodeAs, width, height, err := f.commands.DecodeAsForMedia(ctx, "m1", path)
	if err != nil || decodeAs != "" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"\", nil", decodeAs, err)
	}
	if width != 0 || height != 0 {
		t.Errorf("DecodeAsForMedia() width/height = %d/%d, want 0/0 for a video that is not a real GIF", width, height)
	}
}

// TestDecodeAsForMedia_SanitizesAGenuineGIFAndReportsIt confirms the
// other side: once a video's actual downloaded bytes are a real GIF
// image, DecodeAsForMedia sanitizes them through imagesafe in place,
// reports "gif", and reports the GIF's own real, decoder-confirmed
// canvas size - never whatever width/height the message's own video
// metadata claims - so the UI has something safe to size an
// AnimatedImage's decode from (see
// docs/decisions.md and ui/lib/ImageBounds.js's verifiedDimensions).
func TestDecodeAsForMedia_SanitizesAGenuineGIFAndReportsIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// Width and Height are a lying claim far from the real 1x1 GIF
	// fetched below - the exact shape a security review found could
	// still size the UI's own AnimatedImage decode, since nothing
	// before this fix ever reported the file's own real canvas for a
	// video to use instead.
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[GIF]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, FileName: "run.gif", Animated: true, Width: 2000000, Height: 1},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = onePixelGIF()
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatal(err)
	}

	decodeAs, width, height, err := f.commands.DecodeAsForMedia(ctx, "m1", path)
	if err != nil || decodeAs != "gif" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"gif\", nil", decodeAs, err)
	}
	if width != 1 || height != 1 {
		t.Errorf("DecodeAsForMedia() width/height = %d/%d, want the real 1x1 GIF canvas, never the message's claimed 2000000x1", width, height)
	}

	safe, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(safe, []byte("GIF8")) {
		t.Errorf("cached file no longer starts with a GIF header: %q", safe[:min(len(safe), 16)])
	}
}

// TestDecodeAsForMedia_IgnoresEveryMediaKindButVideo confirms the check
// never even runs for a photo, file or any other kind: only a video can
// ever carry the "animated GIF" ambiguity this guards against.
func TestDecodeAsForMedia_IgnoresEveryMediaKindButVideo(t *testing.T) {
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

	decodeAs, width, height, err := f.commands.DecodeAsForMedia(ctx, "m1", "/does/not/matter")
	if err != nil || decodeAs != "" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"\", nil", decodeAs, err)
	}
	if width != 0 || height != 0 {
		t.Errorf("DecodeAsForMedia() width/height = %d/%d, want 0/0 for a non-video message", width, height)
	}
}

// TestVideoPlayableForMedia_ChecksTheDownloadedBytes covers the three
// outcomes server.mediaFetch depends on: an ordinary video whose
// container and stream headers videosafe accepts is reported
// playable; one that fails that check is reported not playable, with
// no error; and anything that is not a video at all - even one whose
// bytes happen to pass videosafe's own checks - is always reported
// not playable, since the field has no meaning outside a video.
func TestVideoPlayableForMedia_ChecksTheDownloadedBytes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// Two distinct video messages, not one reused for both outcomes:
	// the media cache keys on the message id, so fetching the same id
	// twice would return the first download's cached file again
	// however f.media.data changed in between.
	for _, m := range []domain.Message{
		{ID: "video-good", ConversationID: chat.ID, RemoteID: "1", Text: "[Video]", Created: 1, Media: &domain.Media{Kind: domain.MediaVideo}},
		{ID: "video-bad", ConversationID: chat.ID, RemoteID: "2", Text: "[Video]", Created: 2, Media: &domain.Media{Kind: domain.MediaVideo}},
		{ID: "photo", ConversationID: chat.ID, RemoteID: "3", Text: "[Photo]", Created: 3, Media: &domain.Media{Kind: domain.MediaPhoto}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a well-formed video is reported playable", func(t *testing.T) {
		f.media.data = onePlayableMP4()
		path, _, _, err := f.commands.FetchMedia(ctx, "video-good", false)
		if err != nil {
			t.Fatal(err)
		}

		playable, err := f.commands.VideoPlayableForMedia(ctx, "video-good", path)
		if err != nil || !playable {
			t.Fatalf("VideoPlayableForMedia() = %t, %v, want true, nil", playable, err)
		}
	})

	t.Run("a video whose headers fail the check is not playable", func(t *testing.T) {
		f.media.data = []byte("not a video at all")
		path, _, _, err := f.commands.FetchMedia(ctx, "video-bad", false)
		if err != nil {
			t.Fatal(err)
		}

		playable, err := f.commands.VideoPlayableForMedia(ctx, "video-bad", path)
		if err != nil || playable {
			t.Fatalf("VideoPlayableForMedia() = %t, %v, want false, nil", playable, err)
		}
	})

	t.Run("a non-video message is always not playable", func(t *testing.T) {
		playable, err := f.commands.VideoPlayableForMedia(ctx, "photo", "/does/not/matter")
		if err != nil || playable {
			t.Fatalf("VideoPlayableForMedia() = %t, %v, want false, nil", playable, err)
		}
	})
}

// TestVideoPlayableForMedia_CachesTheVerdictPerFile covers the fix for
// a GIF-as-video bubble re-fetching the same cache file on every
// scroll into view: once VideoPlayableForMedia has computed a verdict
// for path, a second call with the same path, size and modification
// time must serve that same verdict again without re-running
// videosafe's own sample scan. This is proved by silently replacing
// path's own bytes, between the two calls, with something videosafe
// would refuse on its own - keeping the file's size and modification
// time exactly as they were - so only a cache that trusts (path,
// size, modTime) without re-reading the bytes could still report the
// first call's true a second time.
func TestVideoPlayableForMedia_CachesTheVerdictPerFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "video-good", ConversationID: chat.ID, RemoteID: "1", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = onePlayableMP4()
	path, _, _, err := f.commands.FetchMedia(ctx, "video-good", false)
	if err != nil {
		t.Fatal(err)
	}

	playable, err := f.commands.VideoPlayableForMedia(ctx, "video-good", path)
	if err != nil || !playable {
		t.Fatalf("VideoPlayableForMedia() first call = %t, %v, want true, nil", playable, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, int(info.Size())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	playable, err = f.commands.VideoPlayableForMedia(ctx, "video-good", path)
	if err != nil || !playable {
		t.Fatalf("VideoPlayableForMedia() second call, same path/size/modTime but different bytes, = %t, %v, want the cached true, nil", playable, err)
	}
}

// TestAudioPlayableForMedia_ChecksTheDownloadedBytes covers the three
// outcomes server.mediaFetch depends on for audio, mirroring
// TestVideoPlayableForMedia_ChecksTheDownloadedBytes: a voice note
// whose bytes audiosafe accepts is reported playable, the same as an
// ordinary audio message's; one that fails that check is reported not
// playable, with no error; and anything that is neither at all - even
// one whose bytes happen to pass audiosafe's own checks - is always
// reported not playable, since the field has no meaning there.
func TestAudioPlayableForMedia_ChecksTheDownloadedBytes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "voice-good", ConversationID: chat.ID, RemoteID: "1", Text: "[Voice]", Created: 1, Media: &domain.Media{Kind: domain.MediaVoice}},
		{ID: "voice-bad", ConversationID: chat.ID, RemoteID: "2", Text: "[Voice]", Created: 2, Media: &domain.Media{Kind: domain.MediaVoice}},
		{ID: "photo", ConversationID: chat.ID, RemoteID: "3", Text: "[Photo]", Created: 3, Media: &domain.Media{Kind: domain.MediaPhoto}},
		{ID: "audio-good", ConversationID: chat.ID, RemoteID: "4", Text: "[Audio]", Created: 4, Media: &domain.Media{Kind: domain.MediaAudio}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a well-formed voice note is reported playable", func(t *testing.T) {
		f.media.data = audiosafetest.ValidOpusFixture()
		path, _, _, err := f.commands.FetchMedia(ctx, "voice-good", false)
		if err != nil {
			t.Fatal(err)
		}

		playable, err := f.commands.AudioPlayableForMedia(ctx, "voice-good", path)
		if err != nil || !playable {
			t.Fatalf("AudioPlayableForMedia() = %t, %v, want true, nil", playable, err)
		}
	})

	t.Run("a well-formed audio file is reported playable", func(t *testing.T) {
		f.media.data = audiosafetest.ValidOpusFixture()
		path, _, _, err := f.commands.FetchMedia(ctx, "audio-good", false)
		if err != nil {
			t.Fatal(err)
		}

		playable, err := f.commands.AudioPlayableForMedia(ctx, "audio-good", path)
		if err != nil || !playable {
			t.Fatalf("AudioPlayableForMedia() = %t, %v, want true, nil", playable, err)
		}
	})

	t.Run("a voice note whose bytes fail the check is not playable", func(t *testing.T) {
		f.media.data = []byte("not an audio file at all")
		path, _, _, err := f.commands.FetchMedia(ctx, "voice-bad", false)
		if err != nil {
			t.Fatal(err)
		}

		playable, err := f.commands.AudioPlayableForMedia(ctx, "voice-bad", path)
		if err != nil || playable {
			t.Fatalf("AudioPlayableForMedia() = %t, %v, want false, nil", playable, err)
		}
	})

	t.Run("a non-audio message is always not playable", func(t *testing.T) {
		playable, err := f.commands.AudioPlayableForMedia(ctx, "photo", "/does/not/matter")
		if err != nil || playable {
			t.Fatalf("AudioPlayableForMedia() = %t, %v, want false, nil", playable, err)
		}
	})
}

// TestVideoPlayableForMedia_ReportsAMediaFetchErrorWhenTheFileCannotBeRead
// covers failDecodeCheck's own main path, shared by DecodeAsForMedia,
// VideoPlayableForMedia and AudioPlayableForMedia: a check that fails
// not because the bytes are unsafe but because path itself could not
// even be read - here, a cached file gone missing after FetchMedia
// returned it - is classified and logged the same way a failed
// download is, as a *MediaFetchError, never a bare I/O error leaking
// the cache's own path to the UI.
func TestVideoPlayableForMedia_ReportsAMediaFetchErrorWhenTheFileCannotBeRead(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), "gone.mp4")
	_, err := f.commands.VideoPlayableForMedia(ctx, "m1", missing)

	var fetchErr *app.MediaFetchError
	if !errors.As(err, &fetchErr) || fetchErr.Reason != app.MediaDownloadFailed {
		t.Fatalf("VideoPlayableForMedia(unreadable path) = %v, want a *MediaFetchError with reason %q", err, app.MediaDownloadFailed)
	}
	if fetchErr.Error() == "" {
		t.Error("MediaFetchError.Error() is empty, want the underlying read failure's own message")
	}

	if lines := f.logger.take(); len(lines) != 1 {
		t.Errorf("logged lines = %v, want exactly one", lines)
	}
}

// onePixelGIF returns a tiny, real GIF, for a test that needs bytes a
// genuine GIF header check and imagesafe's own sanitizer both actually
// accept, rather than an arbitrary placeholder.
func onePixelGIF() []byte {
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.RGBA{R: 200, G: 100, B: 50, A: 255}})
	img.SetColorIndex(0, 0, 0)

	var buf bytes.Buffer
	_ = gif.Encode(&buf, img, nil)

	return buf.Bytes()
}
