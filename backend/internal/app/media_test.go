package app_test

import (
	"bytes"
	"context"
	"errors"
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

	// A photo's bytes are decoded and re-encoded into a display copy on
	// every fetch (see app.servedPath), so the fake connector's usual
	// stand-in - the message's own remote id, as plain text - is
	// swapped for a real PNG here, unlike the file case below.
	f.media.data = testPNGBytes(t, 2, 2)
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.photo.view.png" {
		t.Fatalf("FetchMedia = %q, %v; want the display copy m1.photo.view.png in the cache", path, err)
	}

	view, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(view, f.media.data) {
		t.Error("display copy holds the original bytes unchanged, want a fresh re-encode")
	}
	assertSamePixels(t, f.media.data, view)

	original, err := os.ReadFile(filepath.Join(f.mediaDir, "m1.photo.jpg"))
	if err != nil || !bytes.Equal(original, f.media.data) {
		t.Errorf("original = %v, %v; want the downloaded bytes kept unchanged alongside the display copy", original, err)
	}

	if _, _, _, err := f.commands.FetchMedia(ctx, "m1", false); err != nil || !slices.Equal(f.media.fetched, []string{"40"}) {
		t.Errorf("second FetchMedia = %v, downloads %v; want the cached copy", err, f.media.fetched)
	}

	// "41", the fake connector's default content for this message, is
	// plain ASCII text, so the file is cached as "m2.file.txt": its own
	// sniffed content, never the sender's claimed "../notes.pdf" (see
	// TestFetchMedia_NamesAFileBySniffedContentNeverTheSendersClaim for
	// the case that matters more - a disguised, dangerous extension).
	// A file, unlike a photo, carries no image check, so the plain text
	// default works here unchanged.
	f.media.data = nil
	if path, _, _, err := f.commands.FetchMedia(ctx, "m2", false); err != nil || filepath.Base(path) != "m2.file.txt" {
		t.Errorf("FetchMedia(file) = %q, %v; want m2.file.txt, named from its own sniffed content", path, err)
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

	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.video.mp4" {
		t.Fatalf("FetchMedia = %q, %v; want m1.video.mp4 in the cache", path, err)
	}
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
		if _, _, _, err := f.commands.FetchMedia(ctx, id, false); !errors.Is(err, want) {
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

	got, _, _, err := f.commands.FetchMedia(ctx, sent.ID, false)
	if err != nil {
		t.Fatalf("FetchMedia() error = %v", err)
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	// The display copy is re-encoded, twice over - once when the
	// attachment itself was sanitized at attach time, again building
	// this display copy from it - so its bytes are never identical to
	// the file originally attached, only its pixels (see
	// docs/decisions.md).
	assertSamePixels(t, want, have)

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

	// The re-download is decoded and re-encoded into a display copy
	// just like any other photo fetch (see app.servedPath), so the
	// fake connector returns a real PNG here rather than its usual
	// remote-id placeholder.
	f.media.data = testPNGBytes(t, 2, 2)
	got, _, _, err := f.commands.FetchMedia(ctx, sent.ID, false)
	if err != nil || filepath.Base(got) != sent.ID+".photo.view.png" {
		t.Fatalf("FetchMedia() = %q, %v; want the service's display copy once the local one is gone", got, err)
	}
	if !slices.Equal(f.media.fetched, []string{"99"}) {
		t.Errorf("fetched = %v, want the service asked for remote id 99", f.media.fetched)
	}

	view, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePixels(t, f.media.data, view)
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
	if _, _, _, err := f.commands.FetchMedia(ctx, "m1", false); !errors.Is(err, f.media.err) {
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
			_, _, _, err := f.commands.FetchMedia(ctx, "m1", false)

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

	_, _, _, err := commands.FetchMedia(ctx, "m1", false)

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

	_, _, _, err := commands.FetchMedia(ctx, "m1", false)

	var mediaErr *app.MediaFetchError
	if !errors.As(err, &mediaErr) || mediaErr.Reason != app.MediaTooLarge {
		t.Fatalf("FetchMedia = %v, want a *MediaFetchError with reason %q", err, app.MediaTooLarge)
	}

	lines := logger.take()
	if len(lines) != 1 || !strings.Contains(lines[0], "reason=too-large") {
		t.Errorf("logged %v, want one line mentioning reason=too-large", lines)
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

	f.media.data = testPNGBytes(t, 2, 2)
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
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

	// The fake connector's retry returns the same unrecognized bytes,
	// so neither the legacy file nor the retry ever passes.
	f.media.data = svg

	var fetchErr *app.MediaFetchError
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
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
	path, _, _, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatalf("FetchMedia() = %v, want the unsafe legacy sticker replaced by a fresh, safe download", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	assertSamePixels(t, f.media.data, got)
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
	f.media.data = svg
	voiceMsg := incoming("e1", "[Voice message]")
	voiceMsg.Media = &domain.Media{Kind: domain.MediaVoice, FileName: "voice.ogg"}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, voiceMsg)

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("Messages() = %+v, %v; want exactly one", messages, err)
	}
	localID := messages[0].ID

	voicePath, _, _, err := f.commands.FetchMedia(ctx, localID, false)
	if err != nil {
		t.Fatalf("FetchMedia(voice) = %v", err)
	}

	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{
		RemoteID: "e1", SenderID: "alex", Text: "look", Media: &domain.Media{Kind: domain.MediaPhoto},
	})

	if _, _, _, err := f.commands.FetchMedia(ctx, localID, false); err == nil {
		t.Fatal("FetchMedia(edited to photo) succeeded, want the unsafe bytes rejected")
	}

	if _, statErr := os.Stat(voicePath); statErr != nil {
		t.Errorf("the voice note's own cached file was touched: %v, want it left alone", statErr)
	}
}

// fakeCache fails every Fetch with err, without ever calling fill, for
// testing how FetchMedia reports a failure the cache itself caused
// rather than one the connector reported.
type fakeCache struct{ err error }

func (c *fakeCache) Fetch(context.Context, string, func(context.Context, string) error) (string, error) {
	return "", c.err
}

// FetchNamed is never exercised by a test using fakeCache; it only ever
// fails the same way Fetch does.
func (c *fakeCache) FetchNamed(context.Context, string, func(context.Context, string) error, func(string) (string, error)) (string, error) {
	return "", c.err
}

// Adopt is never exercised by a test using fakeCache; it only ever fails
// Fetch.
func (c *fakeCache) Adopt(context.Context, string, string) error {
	return c.err
}
