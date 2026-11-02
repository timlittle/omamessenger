package imagesafe_test

import (
	"bytes"
	"image"
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
			out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
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

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
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

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 50000x50000 header, want rejected")
	}
}

// TestSanitizeGIF_RejectsTooManyFrames covers a frame count above the
// cap: no real sticker or message GIF has anywhere near this many.
func TestSanitizeGIF_RejectsTooManyFrames(t *testing.T) {
	src := buildGIF(4, 4, 1100)

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
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
			if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); ok != tc.want {
				t.Errorf("Sanitize(%dx%d x %d frames) ok = %v, want %v", tc.width, tc.height, tc.frames, ok, tc.want)
			}
		})
	}
}

// TestSanitizeGIF_AnimatedGIFRejectsAPhotoSizedCanvas covers the
// animated GIF profile's own per-frame cap, far below a still photo's.
func TestSanitizeGIF_AnimatedGIFRejectsAPhotoSizedCanvas(t *testing.T) {
	src := setGIFDimensions(buildGIF(4, 4, 1), 6000, 6000)

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); ok {
		t.Fatal("Sanitize() = ok for a 6000x6000 animated GIF canvas, want rejected")
	}
}

// TestSanitizeGIF_RejectsFrameExceedingCanvas covers the fix for a
// frame never checked against the logical screen it is supposed to
// fit inside: a tiny declared canvas (here 1x1, as small as a real
// one gets) cannot shrink AnimatedImage's real decode cost just
// because the frame it actually decodes - still 2048x2048, well
// within every per-frame cap on its own - is left completely
// unchecked against that canvas. Before this fix, Sanitize accepted
// this and rated its delay against the fake 1x1 canvas instead,
// which is exactly how a sender could make AnimatedImage decode far
// faster than gifMaxDecodeRate intends.
func TestSanitizeGIF_RejectsFrameExceedingCanvas(t *testing.T) {
	src := setGIFDimensions(buildGIF(2048, 2048, 1), 1, 1)

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); ok {
		t.Fatal("Sanitize() = ok for a 2048x2048 frame on a declared 1x1 canvas, want rejected")
	}
}

// TestSanitizeGIF_CountsEachFrameOwnSizeForTheTotal covers the other
// half of the same fix: the total pixel budget is charged by each
// frame's own real size, not the canvas size repeated once per frame.
// A keyframe followed by many small delta frames - a real encoder's
// own way of animating only the part of the canvas that actually
// changes - decodes far less than canvas-times-frame-count would
// charge it for, and must be accepted on that real cost.
func TestSanitizeGIF_CountsEachFrameOwnSizeForTheTotal(t *testing.T) {
	rects := []image.Rectangle{image.Rect(0, 0, 2000, 2000)} // one full keyframe
	for range 200 {
		rects = append(rects, image.Rect(0, 0, 10, 10)) // 200 tiny delta frames
	}
	src := buildGIFWithFrameRects(2000, 2000, rects)

	// The old canvas-times-frame-count accounting would have charged
	// this 2,000,000*201 = 402,000,000 pixels, just over
	// AnimatedGIF.MaxTotalPixels (400,000,000); its real cost, summing
	// each frame's own size, is only 2,000,000 + 200*100 = 2,020,000.
	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF); !ok {
		t.Fatal("Sanitize() = rejected for a keyframe plus many small delta frames well within the real total pixel budget, want accepted")
	}
}

// TestSanitizeGIF_RejectsTruncatedFile covers a file cut off before
// its trailer.
func TestSanitizeGIF_RejectsTruncatedFile(t *testing.T) {
	src := buildGIF(8, 8, 2)
	truncated := src[:len(src)-5]

	if _, _, _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated file, want rejected")
	}
}

// TestSanitizeGIF_KeepsGlobalColorTable covers the global colour
// table gif.EncodeAll never produces on its own, so a real file using
// one still needs copyGIFScreen to carry it through.
func TestSanitizeGIF_KeepsGlobalColorTable(t *testing.T) {
	src := addGIFGlobalColorTable(buildGIF(8, 8, 1))

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
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

	if _, _, _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated global colour table, want rejected")
	}
}

// TestSanitizeGIF_RejectsUnknownBlock covers a byte where a block
// introducer (an extension, an image or the trailer) must be, but is
// none of them.
func TestSanitizeGIF_RejectsUnknownBlock(t *testing.T) {
	src := buildGIF(4, 4, 1)
	src[13] = 0x99 // first block after the logical screen descriptor

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for an unrecognized block byte, want rejected")
	}
}

// TestSanitizeGIF_RejectsTruncatedExtensionLabel covers an extension
// introducer with no label byte following it.
func TestSanitizeGIF_RejectsTruncatedExtensionLabel(t *testing.T) {
	src := buildGIF(4, 4, 1)
	truncated := append(append([]byte{}, src[:13]...), 0x21)

	if _, _, _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
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

	if _, _, _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a descriptor with no LZW byte, want rejected")
	}
}

// TestSanitizeGIF_RaisesAZeroDelayOnALargeCanvas covers the GIF
// regression.md describes: a sender-chosen delay of zero on every
// frame of a canvas at the animated profile's own cap, which would
// otherwise let AnimatedImage decode that canvas back to back,
// nonstop, for as long as the GIF plays. Sanitize must raise every
// frame's delay so decoding it never runs faster than
// gifMaxDecodeRate pixels of canvas a second.
func TestSanitizeGIF_RaisesAZeroDelayOnALargeCanvas(t *testing.T) {
	src := buildGIFWithDelays(2048, 2048, []int{0, 0, 0, 0, 0})

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}

	delays, err := gifFrameDelays(out)
	if err != nil {
		t.Fatalf("decoding sanitized output: %v", err)
	}

	const wantDelay = 14 // the minimum centisecond delay a 2048x2048 canvas needs to stay under gifMaxDecodeRate
	for i, d := range delays {
		if d < wantDelay {
			t.Errorf("frame %d delay = %d cs, want at least %d", i, d, wantDelay)
		}
	}
}

// TestSanitizeGIF_LeavesASingleFrameUntouched covers a genuine still
// GIF - one frame, no Graphics Control extension at all, the shape
// Go's own gif.Encode produces for a non-animated image - confirming
// Sanitize leaves it exactly as it arrived: it never repeats, so it
// carries none of the decode-rate risk gifMinDelayCS exists to bound,
// and synthesizing a delay for it would only make an already-safe
// file pointlessly different from what was sent.
func TestSanitizeGIF_LeavesASingleFrameUntouched(t *testing.T) {
	src := buildGIFWithDelays(2048, 2048, []int{0})

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.AnimatedGIF)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Equal(out, src) {
		t.Error("Sanitize() rewrote a single, non-animated frame, want it left exactly as it was")
	}
}

// TestSanitizeGIF_LeavesNormalDelaysUnchangedOnASmallCanvas covers an
// ordinary small GIF's own real delay: nothing here should raise or
// otherwise touch a delay the rate bound and the browser convention
// already have no objection to.
func TestSanitizeGIF_LeavesNormalDelaysUnchangedOnASmallCanvas(t *testing.T) {
	src := buildGIFWithDelays(8, 8, []int{10, 10, 10})

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}

	delays, err := gifFrameDelays(out)
	if err != nil {
		t.Fatalf("decoding sanitized output: %v", err)
	}
	for i, d := range delays {
		if d != 10 {
			t.Errorf("frame %d delay = %d cs, want it unchanged at 10", i, d)
		}
	}
}

// assertDecodesGIF fails t if data does not decode as a GIF.
func assertDecodesGIF(t *testing.T, data []byte) {
	t.Helper()

	if _, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
		t.Fatalf("sanitized output does not decode: %v", err)
	}
}

// TestSanitizeGIF_RejectsNonCanonicalGraphicsControl covers the
// differential between Go's own sub-block framing and Qt's GIF
// reader (see docs/decisions.md): Qt always reads exactly five raw
// bytes after a Graphics Control extension's label, ignoring the
// sub-block length byte entirely, so a block shaped any other way
// than one four-byte sub-block makes Qt and this package disagree
// about where the extension ends - the gap a crafted frame-timing
// block can hide unchecked frames in. A shape this package has never
// seen before must be rejected outright, not passed through
// unchanged.
func TestSanitizeGIF_RejectsNonCanonicalGraphicsControl(t *testing.T) {
	tests := map[string][][]byte{
		"first sub-block shorter than four bytes": {{0x00}, {0, 10, 0, 0, 0}},
		"first sub-block longer than four bytes":  {{0x00, 10, 0, 0, 0, 0}},
		"two four-byte sub-blocks":                {{0x00, 10, 0, 0}, {0x00, 20, 0, 0}},
	}

	for name, blocks := range tests {
		t.Run(name, func(t *testing.T) {
			src := insertGIFExtension(buildGIF(8, 8, 2), 0xF9, blocks)

			if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
				t.Fatal("Sanitize() = accepted a non-canonical Graphics Control extension, want rejected")
			}
		})
	}
}

// TestSanitizeGIF_RejectsASecondGraphicsControlForOneFrame covers two
// Graphics Control extensions in a row before the same image block: no
// genuine encoder ever emits this, and this package has no way to
// know which of the two Qt's own reader would actually honour, so it
// is rejected outright rather than guessed at.
func TestSanitizeGIF_RejectsASecondGraphicsControlForOneFrame(t *testing.T) {
	src := insertGIFExtension(buildGIF(8, 8, 2), 0xF9, [][]byte{{0x00, 10, 0, 0}})

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = accepted a second Graphics Control extension for one frame, want rejected")
	}
}

// TestSanitizeGIF_OutputFrameCountMatchesWhatTheLimitsSaw is a
// differential check against the crafted inputs the frame-timing
// differential covers: for every input Sanitize accepts, Go's own
// gif.DecodeAll - the only decoder available to check against in this
// package's own tests - must read back exactly the frame count
// countGIFFrames already bounded, so the canonical form this package
// writes can never carry more frames than its own limits counted.
func TestSanitizeGIF_OutputFrameCountMatchesWhatTheLimitsSaw(t *testing.T) {
	tests := map[string]struct{ width, height, frames int }{
		"a typical message GIF": {480, 270, 60},
		"a single still frame":  {8, 8, 1},
		"many small frames":     {4, 4, 30},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := buildGIF(tc.width, tc.height, tc.frames)

			out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
			if !ok {
				t.Fatal("Sanitize() = rejected, want accepted")
			}

			g, err := gif.DecodeAll(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("decoding sanitized output: %v", err)
			}
			if len(g.Image) != tc.frames {
				t.Errorf("decoded %d frames, want exactly %d", len(g.Image), tc.frames)
			}
		})
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
	f.Add(insertGIFExtension(seed, 0xF9, [][]byte{{0x00}, {0, 10, 0, 0, 0}}))

	f.Fuzz(func(t *testing.T, data []byte) {
		out, _, _, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
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
