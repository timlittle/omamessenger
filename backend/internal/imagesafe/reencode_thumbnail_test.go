package imagesafe_test

// reencode_thumbnail_test.go confirms imagesafe.Thumbnail's own
// DownscaleTo keeps a real sticker at full resolution while still
// shrinking a larger declared size before encoding it: a security
// review found every user of this profile - a message's own embedded
// preview, and a sticker's full, fetched display copy, both of which
// are always drawn small - accepted up to 1024 pixels on a side but
// never shrank before encoding, so a highly compressible but hostile
// 1024x1024 claim paid the full cost of a 1024x1024 re-encode for
// something never shown larger than a sticker ever is (see
// docs/decisions.md).

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// stickerSide is the size WhatsApp and Telegram both actually use for
// a static sticker, the largest real image either service's own
// Thumbnail-profile content (a sticker's display copy; a message's
// own embedded preview is always far smaller) ever legitimately is.
const stickerSide = 512

// TestReencode_ThumbnailKeepsARealStickerAtFullResolution confirms a
// real, sticker-sized image is never downscaled: DownscaleTo must
// only ever shrink, never cost a legitimate sticker any sharpness.
func TestReencode_ThumbnailKeepsARealStickerAtFullResolution(t *testing.T) {
	t.Parallel()

	_, _, width, height, ok := imagesafe.Reencode(buildPNG(stickerSide, stickerSide), imagesafe.Thumbnail)
	if !ok {
		t.Fatal("Reencode() = rejected, want accepted for a real sticker size")
	}
	if width != stickerSide || height != stickerSide {
		t.Errorf("result = %dx%d, want %dx%d unchanged", width, height, stickerSide, stickerSide)
	}
}

// TestReencode_ThumbnailDownscalesAnOversizedClaim confirms a claim
// larger than a real sticker ever is, but still within the profile's
// own accept caps, is shrunk to stickerSide on its longer side before
// being encoded, rather than paying the full encode cost of a size
// nothing legitimate actually needs.
func TestReencode_ThumbnailDownscalesAnOversizedClaim(t *testing.T) {
	t.Parallel()

	_, _, width, height, ok := imagesafe.Reencode(buildPNG(1024, 1024), imagesafe.Thumbnail)
	if !ok {
		t.Fatal("Reencode() = rejected, want accepted: 1024x1024 is still within Thumbnail's own MaxSide and MaxPixels")
	}
	if width != stickerSide || height != stickerSide {
		t.Errorf("result = %dx%d, want downscaled to %dx%d", width, height, stickerSide, stickerSide)
	}
}
