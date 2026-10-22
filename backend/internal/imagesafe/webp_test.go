package imagesafe_test

import (
	"bytes"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSanitizeWebP_StripsMetadataChunks covers the exact class the
// marketplace review flagged for a sticker's thumbnail, applied to a
// sticker's own full image: an extended WebP's ICCP, EXIF and XMP
// chunks, each able to carry an arbitrary, possibly large, payload.
func TestSanitizeWebP_StripsMetadataChunks(t *testing.T) {
	src := buildWebPExtended(64, 64, true, true, true)

	out, ok := imagesafe.Sanitize(src, imagesafe.Thumbnail)
	if !ok {
		t.Fatal("Sanitize() = rejected, want stripped and accepted")
	}
	for _, tag := range []string{"ICCP", "EXIF", "XMP "} {
		if bytes.Contains(out, []byte(tag)) {
			t.Fatalf("output still contains a %s chunk", tag)
		}
	}
}

// TestSanitizeWebP_ClearsVP8XFlags covers VP8X's own flag bits, which
// must no longer claim an ICC, Exif or XMP chunk follows once those
// chunks are dropped.
func TestSanitizeWebP_ClearsVP8XFlags(t *testing.T) {
	src := buildWebPExtended(64, 64, true, true, true)

	out, ok := imagesafe.Sanitize(src, imagesafe.Thumbnail)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}

	vp8x := bytes.Index(out, []byte("VP8X"))
	if vp8x < 0 {
		t.Fatal("output has no VP8X chunk")
	}
	flags := out[vp8x+8]
	if flags&0x2C != 0 {
		t.Fatalf("VP8X flags=%#x, want ICC/Exif/XMP bits cleared", flags)
	}
}

// TestSanitizeWebP_SimpleFormatDimensions covers a plain, non-extended
// WebP: its size comes straight from the VP8 bitstream header, with
// no VP8X chunk at all.
func TestSanitizeWebP_SimpleFormatDimensions(t *testing.T) {
	small := buildWebPSimple(64, 64)
	if _, ok := imagesafe.Sanitize(small, imagesafe.Thumbnail); !ok {
		t.Fatal("Sanitize() = rejected, want accepted for a 64x64 simple WebP")
	}

	huge := buildWebPSimple(9000, 9000)
	if _, ok := imagesafe.Sanitize(huge, imagesafe.Thumbnail); ok {
		t.Fatal("Sanitize() = ok for a 9000x9000 simple WebP, want rejected")
	}
}

// TestSanitizeWebP_RejectsOversizedCanvas covers an extended WebP's
// VP8X chunk declaring a canvas far past profile's cap.
func TestSanitizeWebP_RejectsOversizedCanvas(t *testing.T) {
	src := buildWebPExtended(50000, 50000, false, false, false)

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 50000x50000 canvas, want rejected")
	}
}

// TestSanitizeWebP_RejectsOversizedFrame covers an animated WebP whose
// own canvas is small but one ANMF frame declares a size past the cap.
func TestSanitizeWebP_RejectsOversizedFrame(t *testing.T) {
	src := buildWebPAnimated(64, 64, 1)

	// The frame's own width/height, at offset 6 within its ANMF
	// payload, 24-bit little-endian, one less than the real size.
	anmf := bytes.Index(src, []byte("ANMF"))
	frameHeader := src[anmf+8:]
	frameHeader[6], frameHeader[7], frameHeader[8] = 0xFF, 0xFF, 0xFF

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for an oversized animation frame, want rejected")
	}
}

// TestSanitizeWebP_RejectsTooManyFrames covers a frame count above
// the cap.
func TestSanitizeWebP_RejectsTooManyFrames(t *testing.T) {
	src := buildWebPAnimated(8, 8, 1100)

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for 1100 animation frames, want rejected")
	}
}

// TestSanitizeWebP_CapsTotalDecodedPixels covers an animated WebP whose
// frames together decode to more pixels than the profile allows, the
// same bound an animated GIF gets.
func TestSanitizeWebP_CapsTotalDecodedPixels(t *testing.T) {
	if _, ok := imagesafe.Sanitize(buildWebPAnimated(512, 512, 30), imagesafe.Thumbnail); !ok {
		t.Error("Sanitize() refused a typical animated sticker")
	}
	if _, ok := imagesafe.Sanitize(buildWebPAnimated(1024, 1024, 1000), imagesafe.Thumbnail); ok {
		t.Error("Sanitize() = ok for 1000 frames of a 1024x1024 canvas, want rejected")
	}
}

// TestSanitizeWebP_RejectsTruncatedChunk covers a chunk whose declared
// size runs past the end of the file.
func TestSanitizeWebP_RejectsTruncatedChunk(t *testing.T) {
	src := buildWebPExtended(64, 64, true, false, false)
	truncated := src[:len(src)-5]

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.Thumbnail); ok {
		t.Fatal("Sanitize() = ok for a truncated file, want rejected")
	}
}

// TestSanitizeWebP_KeepsAnimation covers the ANIM and ANMF chunks a
// looping animated sticker needs: dropped by nothing here, only ever
// by the frame-count cap above.
func TestSanitizeWebP_KeepsAnimation(t *testing.T) {
	src := buildWebPAnimated(32, 32, 2)

	out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Contains(out, []byte("ANIM")) || bytes.Count(out, []byte("ANMF")) != 2 {
		t.Fatal("output dropped the animation this sticker needs")
	}
}

// TestSanitizeWebP_LosslessDimensions covers a lossless, non-extended
// WebP, whose size comes from the VP8L bitstream header rather than
// VP8's.
func TestSanitizeWebP_LosslessDimensions(t *testing.T) {
	small := buildWebPLossless(64, 64)
	if _, ok := imagesafe.Sanitize(small, imagesafe.Thumbnail); !ok {
		t.Fatal("Sanitize() = rejected, want accepted for a 64x64 lossless WebP")
	}

	huge := buildWebPLossless(9000, 9000)
	if _, ok := imagesafe.Sanitize(huge, imagesafe.Thumbnail); ok {
		t.Fatal("Sanitize() = ok for a 9000x9000 lossless WebP, want rejected")
	}
}

// TestSanitizeWebP_RejectsBadBitstreamSignatures covers a VP8 chunk
// with the wrong start code and a VP8L chunk with the wrong signature
// byte: neither is trusted to declare a size at all.
func TestSanitizeWebP_RejectsBadBitstreamSignatures(t *testing.T) {
	badVP8 := buildWebPSimple(64, 64)
	vp8 := bytes.Index(badVP8, []byte("VP8 "))
	badVP8[vp8+8+3] = 0x00 // start code's first byte, should be 0x9d

	badVP8L := buildWebPLossless(64, 64)
	vp8l := bytes.Index(badVP8L, []byte("VP8L"))
	badVP8L[vp8l+8] = 0x00 // signature byte, should be 0x2f

	for name, data := range map[string][]byte{"VP8": badVP8, "VP8L": badVP8L} {
		t.Run(name, func(t *testing.T) {
			if _, ok := imagesafe.Sanitize(data, imagesafe.Thumbnail); ok {
				t.Fatalf("Sanitize() = ok for a bad %s signature, want rejected", name)
			}
		})
	}
}

// TestSanitizeWebP_DropsUnknownChunk covers a chunk type this package
// does not recognize at all, dropped the same way an unused private
// chunk in any other format would be.
func TestSanitizeWebP_DropsUnknownChunk(t *testing.T) {
	src := buildWebPExtended(32, 32, false, false, false)
	var withUnknown bytes.Buffer
	withUnknown.Write(src[:12])
	withUnknown.Write(chunk("JUNK", []byte("whatever")))
	withUnknown.Write(src[12:])

	out, ok := imagesafe.Sanitize(withUnknown.Bytes(), imagesafe.Thumbnail)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if bytes.Contains(out, []byte("JUNK")) {
		t.Fatal("output still contains the unrecognized chunk")
	}
}

// TestSanitizeWebP_RejectsTruncatedTopLevelChunk covers a top-level
// chunk header that does not even fit in what remains of the file.
func TestSanitizeWebP_RejectsTruncatedTopLevelChunk(t *testing.T) {
	src := buildWebPSimple(32, 32)
	truncated := src[:14] // past the RIFF/WEBP header, short of a full chunk header

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.Thumbnail); ok {
		t.Fatal("Sanitize() = ok for a truncated chunk header, want rejected")
	}
}

// FuzzSanitizeWebP feeds arbitrary bytes, and small mutations of a
// real extended WebP, to make sure nothing ever panics.
func FuzzSanitizeWebP(f *testing.F) {
	seed := buildWebPExtended(32, 32, true, true, true)
	f.Add(seed)
	f.Add(buildWebPAnimated(16, 16, 2))
	f.Add(buildWebPSimple(16, 16))
	f.Add([]byte{})
	f.Add(seed[:len(seed)-3])

	f.Fuzz(func(t *testing.T, data []byte) {
		out, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
		if !ok && out != nil {
			t.Fatal("rejected input returned non-nil output")
		}
	})
}
