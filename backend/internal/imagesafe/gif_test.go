package imagesafe_test

import (
	"bytes"
	"image/gif"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSanitizeGIF_StripsCommentAndUnknownApplication covers a GIF
// carrying a Comment extension and an Application extension that is
// not the Netscape looping one - exactly where a GIF can smuggle an
// arbitrary text block, such as an embedded XMP packet.
func TestSanitizeGIF_StripsCommentAndUnknownApplication(t *testing.T) {
	original := buildGIF(8, 8, 2)
	withComment := insertGIFExtension(original, 0xFE, [][]byte{bytes.Repeat([]byte{'x'}, 200)})
	withXMP := insertGIFExtension(original, 0xFF, [][]byte{[]byte("XMP DataXMP"), bytes.Repeat([]byte{'y'}, 200)})

	for name, src := range map[string][]byte{"comment": withComment, "application": withXMP} {
		t.Run(name, func(t *testing.T) {
			out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
			if !ok {
				t.Fatal("Sanitize() = rejected, want stripped and accepted")
			}
			if bytes.Contains(out, []byte("DataXMP")) || bytes.Contains(out, bytes.Repeat([]byte{'x'}, 200)) {
				t.Fatal("output still contains the stripped extension's own data")
			}
			assertDecodesGIF(t, out)
		})
	}
}

// TestSanitizeGIF_KeepsNetscapeLoop covers the one Application
// extension sanitizeGIF must not drop: the Netscape looping
// extension AnimatedImage needs to loop at all.
func TestSanitizeGIF_KeepsNetscapeLoop(t *testing.T) {
	src := buildGIF(8, 8, 3)

	out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Contains(out, []byte("NETSCAPE2.0")) {
		t.Fatal("output dropped the Netscape loop extension this animated GIF needs")
	}
}

// TestSanitizeGIF_RejectsOversizedDimensions covers a logical screen
// descriptor declaring a huge width and height.
func TestSanitizeGIF_RejectsOversizedDimensions(t *testing.T) {
	src := setGIFDimensions(buildGIF(4, 4, 1), 50000, 50000)

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 50000x50000 header, want rejected")
	}
}

// TestSanitizeGIF_RejectsTooManyFrames covers a frame count above the
// cap: no real sticker or message GIF has anywhere near this many.
func TestSanitizeGIF_RejectsTooManyFrames(t *testing.T) {
	src := buildGIF(4, 4, 1100)

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for 1100 frames, want rejected")
	}
}

// TestSanitizeGIF_CapsTotalDecodedPixels covers an animation whose
// frames are each within the per-frame cap but which decodes, frame
// after frame, to far more pixels than any real GIF: Qt decodes the
// whole canvas for every frame, so frames times canvas is the work it
// does per loop, and that is what the profile bounds.
func TestSanitizeGIF_CapsTotalDecodedPixels(t *testing.T) {
	tests := map[string]struct {
		width, height, frames int
		want                  bool
	}{
		"a typical message GIF":                         {480, 270, 60, true},
		"many frames of a large canvas":                 {2000, 2000, 200, false},
		"one large frame within the animated GIF limit": {2000, 2000, 1, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := buildGIF(tc.width, tc.height, tc.frames)
			if _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); ok != tc.want {
				t.Errorf("Sanitize(%dx%d x %d frames) ok = %v, want %v", tc.width, tc.height, tc.frames, ok, tc.want)
			}
		})
	}
}

// TestSanitizeGIF_AnimatedGIFRejectsAPhotoSizedCanvas covers the
// animated GIF profile's own per-frame cap, far below a still photo's.
func TestSanitizeGIF_AnimatedGIFRejectsAPhotoSizedCanvas(t *testing.T) {
	src := setGIFDimensions(buildGIF(4, 4, 1), 6000, 6000)

	if _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); ok {
		t.Fatal("Sanitize() = ok for a 6000x6000 animated GIF canvas, want rejected")
	}
}

// TestSanitizeGIF_RejectsTruncatedFile covers a file cut off before
// its trailer.
func TestSanitizeGIF_RejectsTruncatedFile(t *testing.T) {
	src := buildGIF(8, 8, 2)
	truncated := src[:len(src)-5]

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated file, want rejected")
	}
}

// TestSanitizeGIF_KeepsGlobalColorTable covers the global colour
// table gif.EncodeAll never produces on its own, so a real file using
// one still needs copyGIFScreen to carry it through.
func TestSanitizeGIF_KeepsGlobalColorTable(t *testing.T) {
	src := addGIFGlobalColorTable(buildGIF(8, 8, 1))

	out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	assertDecodesGIF(t, out)
	if out[10]&0x80 == 0 {
		t.Fatal("output dropped the global-colour-table flag this test added deliberately")
	}
}

// TestSanitizeGIF_RejectsTruncatedGlobalColorTable covers a
// global-colour-table flag set with too few bytes left for the table
// it promises.
func TestSanitizeGIF_RejectsTruncatedGlobalColorTable(t *testing.T) {
	src := addGIFGlobalColorTable(buildGIF(8, 8, 1))
	truncated := src[:14] // the flag is set, but the 6-byte table is cut short

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated global colour table, want rejected")
	}
}

// TestSanitizeGIF_RejectsUnknownBlock covers a byte where a block
// introducer (an extension, an image or the trailer) must be, but is
// none of them.
func TestSanitizeGIF_RejectsUnknownBlock(t *testing.T) {
	src := buildGIF(4, 4, 1)
	src[13] = 0x99 // first block after the logical screen descriptor

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for an unrecognized block byte, want rejected")
	}
}

// TestSanitizeGIF_RejectsTruncatedExtensionLabel covers an extension
// introducer with no label byte following it.
func TestSanitizeGIF_RejectsTruncatedExtensionLabel(t *testing.T) {
	src := buildGIF(4, 4, 1)
	truncated := append(append([]byte{}, src[:13]...), 0x21)

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated extension, want rejected")
	}
}

// TestSanitizeGIF_RejectsMissingLZWByte covers an image descriptor
// with no byte left for its own LZW minimum code size.
func TestSanitizeGIF_RejectsMissingLZWByte(t *testing.T) {
	src := buildGIF(4, 4, 1)
	descPos := bytes.IndexByte(src, 0x2C)
	lzwPos := descPos + 10
	if packed := src[descPos+9]; packed&0x80 != 0 {
		lzwPos += 3 * (1 << ((packed & 0x07) + 1))
	}
	truncated := src[:lzwPos] // past the descriptor and its colour table, no LZW byte

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a descriptor with no LZW byte, want rejected")
	}
}

// assertDecodesGIF fails t if data does not decode as a GIF.
func assertDecodesGIF(t *testing.T, data []byte) {
	t.Helper()

	if _, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
		t.Fatalf("sanitized output does not decode: %v", err)
	}
}

// FuzzSanitizeGIF feeds arbitrary bytes, and small mutations of a
// real animated GIF, to make sure nothing ever panics and nothing is
// ever returned as "ok" without decoding.
func FuzzSanitizeGIF(f *testing.F) {
	seed := buildGIF(8, 8, 3)
	f.Add(seed)
	f.Add(insertGIFExtension(seed, 0xFE, [][]byte{bytes.Repeat([]byte{1}, 200)}))
	f.Add([]byte{})
	f.Add(seed[:len(seed)-3])

	f.Fuzz(func(t *testing.T, data []byte) {
		out, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
		if !ok {
			if out != nil {
				t.Fatal("rejected input returned non-nil output")
			}
			return
		}
		// sanitizeGIF's own confirmation decode is a cheap
		// DecodeConfig (see finishGIF): walkGIFBlocks checks every
		// block's own framing, but not that its LZW data is valid,
		// so this only re-checks what Sanitize itself promises.
		if _, err := gif.DecodeConfig(bytes.NewReader(out)); err != nil {
			t.Fatalf("accepted output's header no longer parses: %v", err)
		}
	})
}
