package imagesafe_test

// reencode_decodebudget_bypass_test.go confirms estimatedDecodeBytes
// cannot be fooled into charging a progressive JPEG or a 16-bit PNG
// as if it were the cheaper, ordinary shape: a security review found
// the JPEG marker walk treats an "FF 00" byte-stuffed pair as a
// marker with its own length field, letting it skip clean over a real
// SOF2 placed just past one, and the PNG bit depth read assumes IHDR
// sits at a fixed offset right after the signature, which a chunk
// spliced in before it - tolerated by image/png's own decoder, which
// orders only IHDR, PLTE, tRNS, IDAT and IEND against each other -
// quietly defeats. Both fixtures still decode exactly as a real
// image/jpeg or image/png would: these confirm the estimate, not
// decodability, was the gap (see docs/decisions.md).

import (
	"bytes"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestReencode_StillChargesAProgressiveJPEGDisguisedWithStrayFF00
// reproduces the exact shape the review found: a real, genuinely
// progressive JPEG (ten scans, the same fixture
// TestReencode_ProgressiveJPEGCostsMoreThanBaselineAtTheSameSize
// uses) with a stray "FF 00" pair spliced in immediately before its
// SOF2 marker. image/jpeg's own decoder ignores the pair as
// extraneous data and finds SOF2 regardless - the output below still
// decodes - but a walk that instead treats "00" as a marker with a
// length of its own skips clean over the two-byte SOF2 that follows
// and never finds it at all.
func TestReencode_StillChargesAProgressiveJPEGDisguisedWithStrayFF00(t *testing.T) {
	disguised := insertStrayFF00BeforeMarker(readTestdata(t, "jpeg_progressive_10_scans.jpg"), 0xC2)

	profile := imagesafe.Profile{
		MaxSide: 64, MaxPixels: 64 * 64, MaxInputLen: 1 << 20,
		MaxDecodeBytes: 32 * 32 * 8, // between a baseline image's own cheap cost and a progressive one's coefficient-block cost at this size
	}

	out, _, _, _, ok := imagesafe.Reencode(disguised, profile)
	if ok {
		t.Fatal("Reencode() = accepted the disguised progressive JPEG, want rejected: its coefficient blocks should still cost more than the budget allows")
	}
	if out != nil {
		t.Error("Reencode() returned output bytes alongside a rejection, want none")
	}
}

// TestReencode_DisguisedProgressiveJPEGStillDecodesForReal confirms
// insertStrayFF00BeforeMarker's own fixture is not merely rejected by
// coincidence: under a budget wide enough for either shape, it still
// decodes and re-encodes, the same as the undisguised fixture does -
// proving image/jpeg really does tolerate the inserted bytes the way
// its own decode loop's "extraneous data is silently ignored" comment
// says, rather than this fixture accidentally testing a file Reencode
// would have refused for an unrelated reason.
func TestReencode_DisguisedProgressiveJPEGStillDecodesForReal(t *testing.T) {
	disguised := insertStrayFF00BeforeMarker(readTestdata(t, "jpeg_progressive_10_scans.jpg"), 0xC2)

	_, format, width, height, ok := imagesafe.Reencode(disguised, imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected under a generous budget, want accepted: the disguise must not affect real decodability")
	}
	if format != imagesafe.FormatJPEG || width != 32 || height != 32 {
		t.Errorf("format/width/height = %v/%d/%d, want FormatJPEG/32/32, the same as the undisguised fixture", format, width, height)
	}
}

// TestReencode_StillChargesASixteenBitPNGDisguisedWithAChunkBeforeIHDR
// mirrors TestReencode_StillChargesAProgressiveJPEGDisguisedWithStrayFF00
// for the PNG side: a 16-bit-per-channel PNG with one harmless,
// unrecognized chunk spliced in before its IHDR, which image/png's own
// decoder ignores and skips, but a bit depth read at a fixed offset
// from the signature does not: it lands inside what is now the
// inserted chunk's own bytes, never IHDR's real bit depth byte. The
// payload's own ninth byte (index 8) is what a fixed offset-24 read
// finds instead of the real bit depth, once the 8-byte signature and
// the inserted chunk's own 4-byte length and 4-byte type are counted
// off; it is deliberately 1, well within "looks like 8-bit or less",
// so this proves the old offset hides the real depth rather than
// merely landing on some other large byte by chance.
func TestReencode_StillChargesASixteenBitPNGDisguisedWithAChunkBeforeIHDR(t *testing.T) {
	payload := append(bytes.Repeat([]byte{0}, 8), 1)
	disguised := insertPNGChunkBeforeIHDR(build16BitPNG(32, 32), "tEXt", payload)

	profile := imagesafe.Profile{
		MaxSide: 64, MaxPixels: 64 * 64, MaxInputLen: 1 << 20,
		MaxDecodeBytes: 32 * 32 * 8, // between the 8-bit image's own cheap cost and the 16-bit one's higher one
	}

	if _, _, _, _, ok := imagesafe.Reencode(disguised, profile); ok {
		t.Fatal("Reencode() = accepted the disguised 16-bit PNG, want rejected: its own higher per-pixel cost should still exceed the budget")
	}
}

// TestReencode_DisguisedSixteenBitPNGStillDecodesForReal mirrors
// TestReencode_DisguisedProgressiveJPEGStillDecodesForReal for the PNG
// fixture above.
func TestReencode_DisguisedSixteenBitPNGStillDecodesForReal(t *testing.T) {
	payload := append(bytes.Repeat([]byte{0}, 8), 1)
	disguised := insertPNGChunkBeforeIHDR(build16BitPNG(32, 32), "tEXt", payload)

	_, _, width, height, ok := imagesafe.Reencode(disguised, imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected under a generous budget, want accepted: the disguise must not affect real decodability")
	}
	if width != 32 || height != 32 {
		t.Errorf("width/height = %d/%d, want 32/32, the same as the undisguised fixture", width, height)
	}
}
