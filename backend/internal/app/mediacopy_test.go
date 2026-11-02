package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestCopyMedia_WritesAPhotoToTheClipboard confirms a photo message
// whose downloaded bytes genuinely sniff as an image is copied, under
// that sniffed MIME type rather than any claim the message itself
// carries.
func TestCopyMedia_WritesAPhotoToTheClipboard(t *testing.T) {
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

	f.media.data = testPNGBytes(t, 4, 4)
	if err := f.commands.CopyMedia(ctx, "m1"); err != nil {
		t.Fatalf("CopyMedia() = %v", err)
	}

	if len(f.clipboard.written) != 1 {
		t.Fatalf("clipboard.written = %+v, want exactly one write", f.clipboard.written)
	}
	if !strings.HasPrefix(f.clipboard.written[0].mimeType, "image/") {
		t.Errorf("wrote mimeType %q, want an image/* type", f.clipboard.written[0].mimeType)
	}
}

// TestCopyMedia_WritesAVideosPlayCopyToTheClipboard confirms copying a
// video prepares its in-window play copy on demand, through the
// sandboxed transcoder, and copies that - never the raw download - to
// the clipboard (see docs/decisions.md).
func TestCopyMedia_WritesAVideosPlayCopyToTheClipboard(t *testing.T) {
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

	f.media.data = onePlayableMP4()
	if err := f.commands.CopyMedia(ctx, "m1"); err != nil {
		t.Fatalf("CopyMedia() = %v", err)
	}

	if f.transcoder.lastCall() != "video" {
		t.Errorf("transcoder's last call = %q, want \"video\": copying a video must prepare its play copy", f.transcoder.lastCall())
	}
	if len(f.clipboard.written) != 1 {
		t.Fatalf("clipboard.written = %+v, want exactly one write", f.clipboard.written)
	}
	if !strings.HasPrefix(f.clipboard.written[0].mimeType, "video/") {
		t.Errorf("wrote mimeType %q, want a video/* type", f.clipboard.written[0].mimeType)
	}
}

// TestCopyMedia_RejectsAVideoWithNoPlayCopyAvailable confirms a video
// whose play copy the transcoder could not produce is never copied as
// its own raw download instead: there is simply nothing safe to copy.
func TestCopyMedia_RejectsAVideoWithNoPlayCopyAvailable(t *testing.T) {
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

	f.media.data = onePlayableMP4()
	f.transcoder.err = errors.New("ffmpeg exploded")

	if err := f.commands.CopyMedia(ctx, "m1"); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("CopyMedia() = %v, want ErrInvalidInput when no play copy could be built", err)
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written with no play copy available", f.clipboard.written)
	}
}

// TestCopyMedia_RejectsAVideoMessageThatIsActuallyText reproduces the
// security review's own exploit scenario: a contact's modified client
// claims a message is a video, but its downloaded bytes are plain
// text. The old code sniffed the bytes correctly but copied them
// anyway, under whatever MIME type it found; this must now be refused
// rather than putting the bytes on the clipboard under either type.
func TestCopyMedia_RejectsAVideoMessageThatIsActuallyText(t *testing.T) {
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

	f.media.data = []byte(strings.Repeat("this is not a video, just text\n", 100))
	if err := f.commands.CopyMedia(ctx, "m1"); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("CopyMedia() = %v, want ErrInvalidInput", err)
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written", f.clipboard.written)
	}
}

// TestCopyMedia_RejectsAKindItNeverOffersToCopy confirms the server
// side enforces the same allow-list the UI's own copy action is
// limited to (photo, sticker, video) rather than trusting that no
// client ever asks for anything else: a plain file, never offered a
// copy action in this UI at all, is refused here too.
func TestCopyMedia_RejectsAKindItNeverOffersToCopy(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("%PDF-1.4\nreal pdf bytes\n")
	if err := f.commands.CopyMedia(ctx, "m1"); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("CopyMedia() = %v, want ErrInvalidInput", err)
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written", f.clipboard.written)
	}
}

// TestCopyMedia_RejectsAFileOverTheCopyCap confirms a copy this large
// is refused before it ever reaches the clipboard, whatever its
// content: wl-copy holds its own offer in memory for as long as it
// stands, so a cap here is not merely redundant with imagesafe's or
// transcode's own limits.
func TestCopyMedia_RejectsAFileOverTheCopyCap(t *testing.T) {
	t.Parallel()

	// An outgoing attachment's own local copy, not a download: it
	// never passes through the media cache at all (see
	// app.FetchMedia), so this test's own padding is actually what
	// MaxCopyBytes rejects, not some other cache's own, smaller limit.
	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	data := onePlayableMP4()
	padding := make([]byte, app.MaxCopyBytes)
	data = append(data, padding...)
	path := writeTestFile(t, "clip.mp4", data)

	sent, err := f.commands.Send(ctx, "chat", "a clip", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.commands.CopyMedia(ctx, sent.ID); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("CopyMedia() = %v, want ErrInvalidInput", err)
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written", f.clipboard.written)
	}
}

// TestCopyMedia_ReportsAFailedFetch confirms a message with nothing to
// download is reported the same way FetchMedia already reports it,
// never reaching the clipboard at all.
func TestCopyMedia_ReportsAFailedFetch(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.CopyMedia(t.Context(), "missing"); err == nil {
		t.Fatal("CopyMedia() = nil, want an error for a message that does not exist")
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written for a failed fetch", f.clipboard.written)
	}
}

// TestCopyMedia_ReportsAClipboardFailure confirms a wl-copy failure
// reaches the caller rather than being reported as a success.
func TestCopyMedia_ReportsAClipboardFailure(t *testing.T) {
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
	f.media.data = testPNGBytes(t, 4, 4)
	f.clipboard.writeErr = errors.New("wl-copy failed")

	if err := f.commands.CopyMedia(ctx, "m1"); !errors.Is(err, f.clipboard.writeErr) {
		t.Errorf("CopyMedia() = %v, want the clipboard's own error wrapped in", err)
	}
}
