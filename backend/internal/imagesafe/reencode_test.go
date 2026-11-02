package imagesafe_test

// reencode_test.go covers Reencode's own decode-and-re-encode path.
// The one property every case here ultimately checks is that
// omarchy-shell's C decoders never see a byte of the input's own
// compressed pixel data - only a fresh encode this package's own Go
// standard library wrote - which the alpha and format cases below
// demonstrate from different angles, and the decompression-bomb case
// confirms happens before any such decode is ever attempted at all.

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"runtime"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestReencode_OutputNeverContainsInputBytes covers the class this
// package exists to close: a PNG carrying a large chunk of attacker
// data (standing in for whatever a crafted decoder exploit needs to
// place near the real image data) is fully decoded and re-encoded, so
// the sentinel bytes can never survive into the output, whatever they
// actually are.
func TestReencode_OutputNeverContainsInputBytes(t *testing.T) {
	sentinel := bytes.Repeat([]byte("ATTACKER-CONTROLLED-PAYLOAD-"), 100)
	src := insertPNGChunk(buildPNG(8, 8), "zTXt", sentinel)

	out, format, _, _, ok := imagesafe.Reencode(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected, want accepted")
	}
	if bytes.Contains(out, sentinel) {
		t.Fatal("output still contains the input's own bytes")
	}
	if format != imagesafe.FormatJPEG {
		t.Errorf("format = %v, want FormatJPEG for an opaque image", format)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("output does not decode as JPEG: %v", err)
	}
}

// TestReencode_RejectsDeclaredSizeBeforeDecoding covers every format
// this package recognizes: a header declaring a huge width and
// height, far past profile's cap, with no pixel data anywhere near
// that large, must be refused from the header alone - before
// anything tries to decode it, let alone allocate a bitmap for it
// (see TestReencode_RejectsDecompressionBombWithoutAllocating for the
// allocation itself). PNG and GIF declare their size in a container
// field; JPEG and WebP declare it in their own bitstream header.
func TestReencode_RejectsDeclaredSizeBeforeDecoding(t *testing.T) {
	cases := map[string][]byte{
		"png":  setPNGDimensions(buildPNG(4, 4), 100000, 100000),
		"gif":  setGIFDimensions(buildGIF(4, 4, 1), 50000, 50000),
		"webp": buildWebPSimple(9000, 9000),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, _, ok := imagesafe.Reencode(data, imagesafe.FullImage); ok {
				t.Fatalf("Reencode() = accepted for a %s declaring an oversized header, want rejected", name)
			}
		})
	}
}

// TestReencode_RejectsDecompressionBombWithoutAllocating covers the
// reported pixel bomb at the point it matters most: a PNG header
// declaring 50000x50000 (2.5 billion pixels, far past FullImage's 40
// million pixel cap) must never reach a real allocation for that
// canvas, which would need tens of gigabytes. A loose 8 MiB ceiling
// still conclusively proves no such buffer was ever allocated, while
// leaving headroom for ordinary test and runtime noise.
func TestReencode_RejectsDecompressionBombWithoutAllocating(t *testing.T) {
	bomb := setPNGDimensions(buildPNG(2, 2), 50000, 50000)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, _, _, ok := imagesafe.Reencode(bomb, imagesafe.FullImage)
	runtime.ReadMemStats(&after)

	if ok {
		t.Fatal("Reencode() = accepted a 50000x50000 declared PNG, want rejected")
	}

	const ceiling = 8 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew > ceiling {
		t.Errorf("Reencode() allocated %d bytes rejecting a 50000x50000 declared image, want well under %d (no pixel buffer for it)", grew, ceiling)
	}
}

// TestReencode_RejectsWebPInnerChunkLargerThanDeclaredCanvas covers an
// extended (VP8X) WebP whose outer canvas and inner VP8L chunk declare
// different sizes. The pre-decode check above only ever sees the
// canvas - a tiny declared canvas wrapping a chunk that actually
// claims 16384x16384 must still be rejected, and rejected before
// anything allocates a pixel buffer sized for that inner claim, not
// only once the real decode finally finishes and its result is
// measured against profile a second time (see
// docs/decisions.md).
func TestReencode_RejectsWebPInnerChunkLargerThanDeclaredCanvas(t *testing.T) {
	bomb := buildWebPExtendedMismatch(1, 1, 16384, 16384)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, _, _, ok := imagesafe.Reencode(bomb, imagesafe.FullImage)
	runtime.ReadMemStats(&after)

	if ok {
		t.Fatal("Reencode() = accepted a WebP whose inner chunk disagrees with its own declared canvas, want rejected")
	}

	const ceiling = 8 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew > ceiling {
		t.Errorf("Reencode() allocated %d bytes rejecting a mismatched VP8X/VP8L WebP, want well under %d (no pixel buffer for the inner chunk's own claimed size)", grew, ceiling)
	}
}

// TestReencode_OpaqueBecomesJPEG covers the common case: a photo with
// no transparency at all is written back as JPEG, never PNG, so it
// stays small.
func TestReencode_OpaqueBecomesJPEG(t *testing.T) {
	cases := map[string][]byte{
		"png":  buildPNG(32, 32),
		"jpeg": buildJPEG(32, 32),
		"webp": readTestdata(t, "webp_opaque.webp"),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			out, format, width, height, ok := imagesafe.Reencode(data, imagesafe.FullImage)
			if !ok {
				t.Fatalf("Reencode() = rejected, want accepted for an opaque %s", name)
			}
			if format != imagesafe.FormatJPEG {
				t.Errorf("format = %v, want FormatJPEG for an opaque image", format)
			}
			if width <= 0 || height <= 0 {
				t.Errorf("width/height = %d/%d, want both positive", width, height)
			}
			if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
				t.Errorf("output does not decode as JPEG: %v", err)
			}
		})
	}
}

// TestReencode_AlphaBecomesPNG covers a sticker's own cut-out shape:
// any image with real transparency anywhere is written back as PNG,
// which JPEG cannot represent at all.
func TestReencode_AlphaBecomesPNG(t *testing.T) {
	cases := map[string][]byte{
		"png":  buildTransparentPNG(32, 32),
		"webp": readTestdata(t, "webp_alpha.webp"),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			out, format, width, height, ok := imagesafe.Reencode(data, imagesafe.Thumbnail)
			if !ok {
				t.Fatalf("Reencode() = rejected, want accepted for a transparent %s", name)
			}
			if format != imagesafe.FormatPNG {
				t.Errorf("format = %v, want FormatPNG for a transparent image", format)
			}
			if width <= 0 || height <= 0 {
				t.Errorf("width/height = %d/%d, want both positive", width, height)
			}
			if _, err := png.Decode(bytes.NewReader(out)); err != nil {
				t.Errorf("output does not decode as PNG: %v", err)
			}
		})
	}
}

// TestReencode_OpaqueAlphaChannelStillBecomesJPEG covers a PNG whose
// colour type carries an alpha channel but whose every pixel is fully
// opaque: hasAlpha must look at the actual pixels, not just the
// colour model, or every ordinary screenshot would needlessly become
// a larger PNG instead of a JPEG.
func TestReencode_OpaqueAlphaChannelStillBecomesJPEG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	_, format, _, _, ok := imagesafe.Reencode(buf.Bytes(), imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected, want accepted")
	}
	if format != imagesafe.FormatJPEG {
		t.Errorf("format = %v, want FormatJPEG for an NRGBA image with every pixel opaque", format)
	}
}

// TestReencode_RefusesAGIFOutright covers the format this package
// used to reduce to its first frame and now cannot decode at all:
// every connector routes a real GIF animation through the sandboxed
// video path instead (see docs/decisions.md), so this package no
// longer registers a GIF decoder, and genuine GIF bytes reaching
// Reencode fail the same way any other unrecognised format does.
func TestReencode_RefusesAGIFOutright(t *testing.T) {
	red := buildSolidColorGIF(8, 8, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255})

	if _, _, _, _, ok := imagesafe.Reencode(red, imagesafe.Thumbnail); ok {
		t.Fatal("Reencode() = accepted for a real GIF, want refused outright")
	}
}

// TestReencode_RefusesAnimatedWebP covers the one format this
// package cannot safely reduce to a still image at all:
// golang.org/x/image/webp has no decoder for an animated WebP, so
// Reencode must refuse it outright rather than, say, silently
// treating the container's own declared canvas as empty pixels.
func TestReencode_RefusesAnimatedWebP(t *testing.T) {
	data := readTestdata(t, "webp_animated.webp")

	if _, _, _, _, ok := imagesafe.Reencode(data, imagesafe.FullImage); ok {
		t.Fatal("Reencode() = accepted an animated WebP, want rejected")
	}
}

// TestSanitize_RejectsZeroDeclaredDimension covers withinCaps's own
// floor: a width or height of zero, which no real image ever has but
// a crafted header can still declare.
func TestSanitize_RejectsZeroDeclaredDimension(t *testing.T) {
	data := setPNGDimensions(buildPNG(4, 4), 0, 100)

	if _, _, _, ok := imagesafe.Sanitize(data, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = accepted a zero-width declared image, want rejected")
	}
}

// TestSanitize_RejectsProductOverMaxPixelsEvenUnderMaxSide covers the
// shape MaxSide alone cannot catch: both sides individually within
// MaxSide, but their product still over MaxPixels - only possible
// when a profile's own MaxPixels is set below MaxSide squared, unlike
// either of this package's own named profiles.
func TestSanitize_RejectsProductOverMaxPixelsEvenUnderMaxSide(t *testing.T) {
	data := buildPNG(90, 90) // 8,100 pixels
	profile := imagesafe.Profile{MaxSide: 100, MaxPixels: 8000, MaxInputLen: 1 << 20}

	if _, _, _, ok := imagesafe.Sanitize(data, profile); ok {
		t.Fatal("Sanitize() = accepted, want rejected: both sides are under MaxSide but their product is over MaxPixels")
	}
}

// TestReencode_RejectsOverInputLenBeforeFormatDetection covers the
// cheapest rejection of all: data larger than profile ever allows is
// refused before this package even looks at its magic bytes.
func TestReencode_RejectsOverInputLenBeforeFormatDetection(t *testing.T) {
	data := buildPNG(4, 4)
	profile := imagesafe.Profile{MaxSide: 8, MaxPixels: 64, MaxInputLen: int64(len(data) - 1)}

	if _, format, _, _, ok := imagesafe.Reencode(data, profile); ok || format != imagesafe.FormatUnknown {
		t.Fatalf("Reencode() = (format=%v, ok=%v), want (FormatUnknown, false) for data over MaxInputLen", format, ok)
	}
}

// TestReencode_RejectsJPEGWithTooManyScans covers the CPU cost Go's
// own progressive JPEG decoder has no limit on: each start-of-scan
// marker makes it re-walk every block of the image again, so a file
// declaring 101 of them must be refused before that decode ever
// runs. The fixture is a real, otherwise-valid progressive JPEG -
// Go's decoder accepts it fine without this cap - carrying 91 extra
// repeats of one genuine, tiny scan, so this proves the new count
// check itself rejects it, not some unrelated decode failure (see
// docs/decisions.md for the measured cost this closes).
func TestReencode_RejectsJPEGWithTooManyScans(t *testing.T) {
	data := readTestdata(t, "jpeg_progressive_101_scans.jpg")

	if _, _, _, _, ok := imagesafe.Reencode(data, imagesafe.FullImage); ok {
		t.Fatal("Reencode() = accepted a JPEG with 101 scan markers, want rejected")
	}
}

// TestReencode_AcceptsRealisticProgressiveScanCount covers the cap's
// other edge: a real photo encoded progressively can easily carry
// ten genuine scans, well under the cap, and must still decode and
// re-encode normally.
func TestReencode_AcceptsRealisticProgressiveScanCount(t *testing.T) {
	data := readTestdata(t, "jpeg_progressive_10_scans.jpg")

	out, format, width, height, ok := imagesafe.Reencode(data, imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected a progressive JPEG with 10 scans, want accepted")
	}
	if format != imagesafe.FormatJPEG {
		t.Errorf("format = %v, want FormatJPEG", format)
	}
	if width != 32 || height != 32 {
		t.Errorf("width/height = %d/%d, want 32/32", width, height)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("output does not decode as JPEG: %v", err)
	}
}

// TestReencode_RejectsOverDecodeByteBudget covers the memory-budget
// check: a profile whose MaxDecodeBytes is far below even a tiny
// image's own estimate must refuse it before the real decode runs,
// the same proof-of-no-allocation
// TestReencode_RejectsDecompressionBombWithoutAllocating already gives
// the size caps.
func TestReencode_RejectsOverDecodeByteBudget(t *testing.T) {
	data := buildPNG(100, 100)
	profile := imagesafe.Profile{
		MaxSide: 1000, MaxPixels: 1_000_000, MaxInputLen: int64(len(data)) + 1,
		MaxDecodeBytes: 1,
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, _, _, ok := imagesafe.Reencode(data, profile)
	runtime.ReadMemStats(&after)

	if ok {
		t.Fatal("Reencode() = accepted, want rejected: MaxDecodeBytes is far below this image's own estimate")
	}

	const ceiling = 8 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew > ceiling {
		t.Errorf("Reencode() allocated %d bytes rejecting a 100x100 image over budget, want well under %d", grew, ceiling)
	}
}

// TestReencode_ZeroMaxDecodeBytesMeansNoLimit covers the field's own
// zero-value convention, the same one MaxAspectRatio already
// documents: a profile that never sets MaxDecodeBytes at all - every
// ad hoc Profile literal built before this field existed - must keep
// behaving exactly as it always has.
func TestReencode_ZeroMaxDecodeBytesMeansNoLimit(t *testing.T) {
	data := buildPNG(100, 100)
	profile := imagesafe.Profile{MaxSide: 1000, MaxPixels: 1_000_000, MaxInputLen: int64(len(data)) + 1}

	if _, _, _, _, ok := imagesafe.Reencode(data, profile); !ok {
		t.Fatal("Reencode() = rejected, want accepted: MaxDecodeBytes 0 must mean no limit")
	}
}

// TestReencode_ProgressiveJPEGCostsMoreThanBaselineAtTheSameSize covers
// the reason estimatedDecodeBytes exists at all: a progressive JPEG
// keeps every block's coefficients live across every one of its
// scans, so it must be charged more per pixel than a baseline JPEG of
// the exact same declared size. The fixture is a real, genuinely
// progressive JPEG (ten real scans, already used elsewhere in this
// file for the scan-count cap), not a baseline file relabelled, so
// this proves the SOF marker this package reads actually tells the
// two apart.
func TestReencode_ProgressiveJPEGCostsMoreThanBaselineAtTheSameSize(t *testing.T) {
	progressive := readTestdata(t, "jpeg_progressive_10_scans.jpg") // 32x32
	baseline := buildJPEG(32, 32)

	profile := imagesafe.Profile{
		MaxSide: 64, MaxPixels: 64 * 64, MaxInputLen: 1 << 20,
		MaxDecodeBytes: 32 * 32 * 8, // between a baseline image's own cheap cost and a progressive one's coefficient-block cost at this size
	}

	if _, _, _, _, ok := imagesafe.Reencode(baseline, profile); !ok {
		t.Fatal("Reencode() = rejected the baseline JPEG, want accepted: its estimate should sit well under the budget")
	}
	if _, _, _, _, ok := imagesafe.Reencode(progressive, profile); ok {
		t.Fatal("Reencode() = accepted the progressive JPEG, want rejected: its coefficient blocks should cost more than the budget allows")
	}
}

// TestReencode_SixteenBitPNGCostsMoreThanEightBitAtTheSameSize covers
// the PNG side of estimatedDecodeBytes: a 16-bit-per-channel image
// costs more to decode than an ordinary 8-bit one of the same
// declared size, read from the IHDR bit depth byte alone.
func TestReencode_SixteenBitPNGCostsMoreThanEightBitAtTheSameSize(t *testing.T) {
	eightBit := buildPNG(32, 32)
	sixteenBit := build16BitPNG(32, 32)

	profile := imagesafe.Profile{
		MaxSide: 64, MaxPixels: 64 * 64, MaxInputLen: 1 << 20,
		MaxDecodeBytes: 32 * 32 * 8, // between the 8-bit image's own cheap cost and the 16-bit one's higher one
	}

	if _, _, _, _, ok := imagesafe.Reencode(eightBit, profile); !ok {
		t.Fatal("Reencode() = rejected the 8-bit PNG, want accepted: its estimate should sit well under the budget")
	}
	if _, _, _, _, ok := imagesafe.Reencode(sixteenBit, profile); ok {
		t.Fatal("Reencode() = accepted the 16-bit PNG, want rejected: its own higher per-pixel cost should exceed the budget")
	}
}

// TestReencode_DimensionsMatchesItsOwnOutput covers Dimensions, the
// cheap header-only read a caller uses on a cache hit instead of
// paying for Reencode's own full decode again: it must agree with
// the width and height Reencode itself just measured by decoding.
func TestReencode_DimensionsMatchesItsOwnOutput(t *testing.T) {
	out, _, width, height, ok := imagesafe.Reencode(buildPNG(40, 30), imagesafe.FullImage)
	if !ok {
		t.Fatal("Reencode() = rejected, want accepted")
	}

	gotWidth, gotHeight, ok := imagesafe.Dimensions(bytes.NewReader(out))
	if !ok {
		t.Fatal("Dimensions() = rejected Reencode's own output")
	}
	if gotWidth != width || gotHeight != height {
		t.Errorf("Dimensions() = %dx%d, want %dx%d (Reencode's own measurement)", gotWidth, gotHeight, width, height)
	}
}

// TestDimensions_RejectsGarbage covers data that is not any
// recognized image at all.
func TestDimensions_RejectsGarbage(t *testing.T) {
	if _, _, ok := imagesafe.Dimensions(bytes.NewReader([]byte("not an image"))); ok {
		t.Fatal("Dimensions() = ok for plain text, want rejected")
	}
}

// TestOutputFormat_ExtensionAndMIME covers every format Reencode can
// ever write, plus FormatUnknown, which a caller must never use to
// name or label anything.
func TestOutputFormat_ExtensionAndMIME(t *testing.T) {
	cases := map[imagesafe.OutputFormat]struct {
		ext, mime string
	}{
		imagesafe.FormatJPEG:    {".jpg", "image/jpeg"},
		imagesafe.FormatPNG:     {".png", "image/png"},
		imagesafe.FormatUnknown: {"", ""},
	}

	for format, want := range cases {
		if got := format.Extension(); got != want.ext {
			t.Errorf("%v.Extension() = %q, want %q", format, got, want.ext)
		}
		if got := format.MIME(); got != want.mime {
			t.Errorf("%v.MIME() = %q, want %q", format, got, want.mime)
		}
	}
}

// FuzzReencode feeds arbitrary bytes, and small mutations of real
// images in every format this package recognizes, to make sure
// Reencode never panics, and that whatever it accepts both decodes
// cleanly with Go and stays within the profile it was checked
// against - the two guarantees every caller relies on.
func FuzzReencode(f *testing.F) {
	f.Add(buildPNG(8, 8))
	f.Add(insertPNGChunk(buildPNG(8, 8), "zTXt", bytes.Repeat([]byte{1}, 2048)))
	f.Add(setPNGDimensions(buildPNG(8, 8), 50000, 50000))
	f.Add(buildJPEG(8, 8))
	f.Add(insertFakeMarkersWithFill(buildJPEG(64, 64), 0xDA, 1, 500_000)) // the old fill-byte bypass; Go's own decoder must still handle this safely
	f.Add(buildGIF(8, 8, 3))
	f.Add(buildWebPSimple(8, 8))
	f.Add(buildWebPAnimated(8, 8, 2))
	f.Add(buildWebPExtendedMismatch(1, 1, 16384, 16384)) // the VP8X/inner-chunk size mismatch; x/image must never allocate or decode past its own caps over this
	f.Add([]byte{})
	f.Add([]byte("not an image at all"))

	f.Fuzz(func(t *testing.T, data []byte) {
		out, format, width, height, ok := imagesafe.Reencode(data, imagesafe.FullImage)
		if !ok {
			if out != nil || format != imagesafe.FormatUnknown || width != 0 || height != 0 {
				t.Fatal("rejected input returned a non-zero result")
			}
			return
		}

		img, _, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("accepted output does not decode: %v", err)
		}
		if b := img.Bounds(); b.Dx() != width || b.Dy() != height {
			t.Fatalf("accepted output decodes to %dx%d, want the reported %dx%d", b.Dx(), b.Dy(), width, height)
		}
		if int64(width) > int64(imagesafe.FullImage.MaxSide) || int64(height) > int64(imagesafe.FullImage.MaxSide) {
			t.Fatalf("accepted output is %dx%d, over FullImage's own MaxSide", width, height)
		}
	})
}

// readTestdata reads name from this package's own testdata directory,
// failing t if it cannot.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}

	return data
}
