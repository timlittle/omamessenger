// attachment_sniff_test.go covers attachment.go's own byte-sniffing and
// thumbnail paths - what Send decides an attachment's kind is from its
// actual bytes, and the edge cases around building its dimensions and
// preview - split out of attachment_test.go to keep that file within
// the project's own line-length guideline.
package app_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSend_WithAttachment_SniffsAVideoFromItsOwnBytes confirms an
// attachment whose first bytes are a video container's magic header -
// never its file extension - is stored as a video, with no dimensions
// or thumbnail: those are built only for a photo (see describeAttachment).
func TestSend_WithAttachment_SniffsAVideoFromItsOwnBytes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	// The four-byte EBML header every Matroska/WebM file starts with;
	// http.DetectContentType keys on exactly this, the same way it
	// would for a real recording, with no further structure needed.
	path := writeTestFile(t, "clip.webm", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 32)...))

	m, err := f.commands.Send(ctx, "chat", "a clip", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media == nil || m.Media.Kind != domain.MediaVideo {
		t.Fatalf("Media = %+v, want a video", m.Media)
	}
	if m.Media.Width != 0 || m.Media.Height != 0 || m.Media.Thumb != "" {
		t.Errorf("Media = %+v, want no dimensions or thumbnail for a video", m.Media)
	}
}

// TestSend_WithAttachment_ExtremeAspectRatioStillGetsADecodableThumb
// covers shrink's own floor: a photo so much wider than it is tall
// that scaling its short side down by the same factor as its long one
// would round to zero pixels still gets a real, decodable thumbnail,
// because scaledSize never returns less than one.
func TestSend_WithAttachment_ExtremeAspectRatioStillGetsADecodableThumb(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "wide.png", 2000, 1)

	m, err := f.commands.Send(ctx, "chat", "wide", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media.Thumb == "" {
		t.Fatal("Media.Thumb is empty, want a thumbnail even for an extreme aspect ratio")
	}

	raw, err := base64.StdEncoding.DecodeString(m.Media.Thumb)
	if err != nil {
		t.Fatalf("Thumb is not valid base64: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Thumb is not a decodable image: %v", err)
	}
	if cfg.Height != 1 {
		t.Errorf("Thumb height = %d, want the short side floored to 1 pixel, not 0", cfg.Height)
	}
}

// TestSend_ReusesAnAttachmentWhoseBytesNoLongerDecodeAsAnImage covers
// reuseAttachment's own tolerance for a stored file that sniffs as a
// photo by its header but no longer decodes as one: media.attach and
// media.paste already sanitize a photo before it ever reaches the
// outgoing area, so reuseAttachment itself never re-sanitizes, but it
// must still cope rather than panic if that copy were ever corrupted
// on disk before the message that reuses it is sent. Width, height and
// the thumbnail all fall back to their empty values; the attachment
// itself is still sent.
func TestSend_ReusesAnAttachmentWhoseBytesNoLongerDecodeAsAnImage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	// A real PNG header followed by garbage: sniffKind calls this a
	// photo from the header alone, but neither image.Decode nor
	// image.DecodeConfig can read any further.
	corrupt := append([]byte("\x89PNG\r\n\x1a\n"), []byte("not a real png body at all")...)
	path, err := f.outgoing.Store(ctx, "m_pending", "photo.png", bytes.NewReader(corrupt))
	if err != nil {
		t.Fatal(err)
	}

	m, err := f.commands.Send(ctx, "chat", "look", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media == nil || m.Media.Kind != domain.MediaPhoto {
		t.Fatalf("Media = %+v, want it still classified as a photo from its header", m.Media)
	}
	if m.Media.Width != 0 || m.Media.Height != 0 {
		t.Errorf("Media dimensions = %dx%d, want 0x0 for an undecodable photo", m.Media.Width, m.Media.Height)
	}
	if m.Media.Thumb != "" {
		t.Errorf("Media.Thumb = %q, want none for an undecodable photo", m.Media.Thumb)
	}
}
