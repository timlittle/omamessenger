package app_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestFetchMedia_DownloadsOnceIntoTheCache(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	photo := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	file := domain.Message{
		ID: "m2", ConversationID: chat.ID, RemoteID: "41", Text: "[File]", Created: 2,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "../notes.pdf"},
	}
	for _, m := range []domain.Message{photo, file} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.jpg" {
		t.Fatalf("FetchMedia = %q, %v; want m1.jpg in the cache", path, err)
	}

	if got, _ := os.ReadFile(path); string(got) != "40" {
		t.Errorf("cached %q, want the downloaded media", got)
	}

	if _, err := f.commands.FetchMedia(ctx, "m1", false); err != nil || !slices.Equal(f.media.fetched, []string{"40"}) {
		t.Errorf("second FetchMedia = %v, downloads %v; want the cached copy", err, f.media.fetched)
	}

	// "41", the fake connector's default content for this message, is
	// plain ASCII text, so the file is cached as "m2.txt": its own
	// sniffed content, never the sender's claimed "../notes.pdf" (see
	// TestFetchMedia_NamesAFileBySniffedContentNeverTheSendersClaim for
	// the case that matters more - a disguised, dangerous extension).
	if path, err := f.commands.FetchMedia(ctx, "m2", false); err != nil || filepath.Base(path) != "m2.txt" {
		t.Errorf("FetchMedia(file) = %q, %v; want m2.txt, named from its own sniffed content", path, err)
	}
}

// TestFetchMedia_CachesEveryVideoWithAnMp4Extension confirms the cache
// never again picks a ".gif" name from a message's own filename or
// "animated" flag (the bug behind a disguised video bypassing the image
// sanitizer): every MediaVideo message, whatever it claims about
// itself, is cached as "<id>.mp4". A real GIF's actual decode decision
// is DecodeAsForMedia's own job, not the cache's.
func TestFetchMedia_CachesEveryVideoWithAnMp4Extension(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	gif := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[GIF]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, FileName: "run.gif", Animated: true},
	}
	if _, _, err := f.store.AddMessage(ctx, gif); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.mp4" {
		t.Fatalf("FetchMedia = %q, %v; want m1.mp4 in the cache", path, err)
	}
}

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
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatal(err)
	}

	decodeAs, err := f.commands.DecodeAsForMedia(ctx, "m1", path)
	if err != nil || decodeAs != "" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"\", nil", decodeAs, err)
	}
}

// TestDecodeAsForMedia_SanitizesAGenuineGIFAndReportsIt confirms the
// other side: once a video's actual downloaded bytes are a real GIF
// image, DecodeAsForMedia sanitizes them through imagesafe in place and
// reports "gif", so the UI can trust decoding it as an animated image.
func TestDecodeAsForMedia_SanitizesAGenuineGIFAndReportsIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[GIF]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, FileName: "run.gif", Animated: true},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = onePixelGIF()
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatal(err)
	}

	decodeAs, err := f.commands.DecodeAsForMedia(ctx, "m1", path)
	if err != nil || decodeAs != "gif" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"gif\", nil", decodeAs, err)
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

	decodeAs, err := f.commands.DecodeAsForMedia(ctx, "m1", "/does/not/matter")
	if err != nil || decodeAs != "" {
		t.Fatalf("DecodeAsForMedia() = %q, %v; want \"\", nil", decodeAs, err)
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
		path, err := f.commands.FetchMedia(ctx, "video-good", false)
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
		path, err := f.commands.FetchMedia(ctx, "video-bad", false)
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

func TestFetchMedia_Rejects(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "plain", ConversationID: chat.ID, RemoteID: "1", Text: "hi", Created: 1},
		{ID: "link", ConversationID: chat.ID, RemoteID: "2", Text: "x.io", Created: 2, Media: &domain.Media{Kind: domain.MediaLink}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for id, want := range map[string]error{
		" ":       app.ErrInvalidInput,
		"plain":   app.ErrInvalidInput,
		"link":    app.ErrInvalidInput,
		"missing": domain.ErrNotFound,
	} {
		if _, err := f.commands.FetchMedia(ctx, id, false); !errors.Is(err, want) {
			t.Errorf("FetchMedia(%q) = %v, want %v", id, err, want)
		}
	}
}

func TestFetchMedia_OutgoingPhotoUsesTheLocalCopyWhileStillPending(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 2, 2)

	sent, err := f.commands.Send(ctx, "chat", "look", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
	if err != nil || sent.Status != domain.StatusPending || sent.RemoteID != "" {
		t.Fatalf("Send() = %+v, %v; want it still pending with no remote id", sent, err)
	}

	got, err := f.commands.FetchMedia(ctx, sent.ID, false)
	if err != nil {
		t.Fatalf("FetchMedia() error = %v", err)
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(got)
	if err != nil || !bytes.Equal(have, want) {
		t.Errorf("FetchMedia() = %q, %v; want the same bytes as the attachment sent", got, err)
	}

	if len(f.media.fetched) != 0 {
		t.Errorf("downloaded from the service %v, want the local copy used instead", f.media.fetched)
	}
}

func TestFetchMedia_OutgoingPhotoFallsBackToDownloadIfTheLocalCopyIsGone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 2, 2)

	sent, err := f.commands.Send(ctx, "chat", "look", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if err := os.Remove(f.outgoing.Path(sent.ID, "photo.png")); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetMessageRemoteID(ctx, sent.ID, "99"); err != nil {
		t.Fatal(err)
	}

	got, err := f.commands.FetchMedia(ctx, sent.ID, false)
	if err != nil || filepath.Base(got) != sent.ID+".jpg" {
		t.Fatalf("FetchMedia() = %q, %v; want the service's copy once the local one is gone", got, err)
	}
	if !slices.Equal(f.media.fetched, []string{"99"}) {
		t.Errorf("fetched = %v, want the service asked for remote id 99", f.media.fetched)
	}
}

func TestFetchMedia_ReportsADownloadFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]",
		Created: 1, Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.err = errors.New("offline")
	if _, err := f.commands.FetchMedia(ctx, "m1", false); !errors.Is(err, f.media.err) {
		t.Errorf("FetchMedia = %v, want the download's error", err)
	}
}

// TestFetchMedia_LogsAndCategorizesEveryFailureReason confirms a failed
// download is never silent: it is logged with one of the safe reason
// categories docs/decisions.md promises, and returned as a
// *app.MediaFetchError carrying the same category, for the UI's
// "Unavailable" tooltip to show.
func TestFetchMedia_LogsAndCategorizesEveryFailureReason(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		connectorErr error
		wantReason   app.MediaFetchReason
	}{
		"a reference the service never saved":      {domain.ErrNotFound, app.MediaNotFound},
		"media the phone no longer has":            {domain.ErrMediaExpired, app.MediaExpired},
		"a downloaded file that failed to decrypt": {domain.ErrMediaDecryptFailed, app.MediaDecryptFailed},
		"a declared or actual size over the limit": {connector.ErrMediaTooLarge, app.MediaTooLarge},
		"a fetch that ran out of time":             {context.DeadlineExceeded, app.MediaTimedOut},
		"a fetch cancelled with the request":       {context.Canceled, app.MediaTimedOut},
		"a plain network or server failure":        {errors.New("offline"), app.MediaDownloadFailed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
			if _, _, err := f.store.AddMessage(ctx, domain.Message{
				ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Voice message]",
				Created: 1, Media: &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg"},
			}); err != nil {
				t.Fatal(err)
			}

			f.media.err = tt.connectorErr
			_, err := f.commands.FetchMedia(ctx, "m1", false)

			var mediaErr *app.MediaFetchError
			if !errors.As(err, &mediaErr) || mediaErr.Reason != tt.wantReason {
				t.Fatalf("FetchMedia = %v, want a *MediaFetchError with reason %q", err, tt.wantReason)
			}

			lines := f.logger.take()
			if len(lines) != 1 {
				t.Fatalf("logged %d lines, want exactly 1", len(lines))
			}

			want := "reason=" + string(tt.wantReason)
			if !strings.Contains(lines[0], want) || !strings.Contains(lines[0], "kind=voice") || !strings.Contains(lines[0], "service=whatsapp") {
				t.Errorf("logged %q, want it to mention %q, kind=voice and service=whatsapp", lines[0], want)
			}
		})
	}
}

// TestFetchMedia_ReportsACacheFailureWithoutCallingTheConnector confirms
// a cache failure that never even reaches the connector, such as a full
// disk, is still reported with its own reason, rather than being
// mistaken for a download failure.
func TestFetchMedia_ReportsACacheFailureWithoutCallingTheConnector(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Voice message]",
		Created: 1, Media: &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg"},
	}); err != nil {
		t.Fatal(err)
	}

	logger := &fakeLogger{}
	commands, _ := app.New(app.Deps{
		Store: f.store, Media: f.media, Cache: &fakeCache{err: errors.New("cache: disk full")}, Logger: logger,
	})

	_, err := commands.FetchMedia(ctx, "m1", false)

	var mediaErr *app.MediaFetchError
	if !errors.As(err, &mediaErr) || mediaErr.Reason != app.MediaCacheFailed {
		t.Fatalf("FetchMedia = %v, want a *MediaFetchError with reason %q", err, app.MediaCacheFailed)
	}
	if len(f.media.fetched) != 0 {
		t.Errorf("fetched = %v, want the connector never called for a cache failure", f.media.fetched)
	}

	lines := logger.take()
	if len(lines) != 1 || !strings.Contains(lines[0], "reason=cache") {
		t.Errorf("logged %v, want one line mentioning reason=cache", lines)
	}
}

// TestFetchMedia_ReportsATooLargeFileTheCacheRefusedAfterDownloading
// confirms a download that completed - the connector was happy with
// it, and every one of its own size checks passed - but whose own
// bytes alone exceed the cache's own, separate limit (see
// cache.ErrTooLargeForCache) is still reported as too large, not a
// generic cache failure: the UI shows the same "too large" reason
// whichever layer actually refused it.
func TestFetchMedia_ReportsATooLargeFileTheCacheRefusedAfterDownloading(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "huge.bin"},
	}); err != nil {
		t.Fatal(err)
	}

	const limit = 1024
	f.media.data = bytes.Repeat([]byte{0x42}, limit+1)
	logger := &fakeLogger{}
	commands, _ := app.New(app.Deps{
		Store: f.store, Media: f.media, Cache: cache.New(filepath.Join(t.TempDir(), "media"), limit), Logger: logger,
	})

	_, err := commands.FetchMedia(ctx, "m1", false)

	var mediaErr *app.MediaFetchError
	if !errors.As(err, &mediaErr) || mediaErr.Reason != app.MediaTooLarge {
		t.Fatalf("FetchMedia = %v, want a *MediaFetchError with reason %q", err, app.MediaTooLarge)
	}

	lines := logger.take()
	if len(lines) != 1 || !strings.Contains(lines[0], "reason=too-large") {
		t.Errorf("logged %v, want one line mentioning reason=too-large", lines)
	}
}
