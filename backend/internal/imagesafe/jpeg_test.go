package imagesafe_test

import (
	"bytes"
	"image/jpeg"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSanitizeJPEG_StripsAppAndComment covers a JPEG carrying a large
// EXIF (APP1), ICC profile (APP2) or comment segment: none of them
// affect the pixels a JPEG decoder produces, so the sanitized output
// must drop them but still decode to the same image.
func TestSanitizeJPEG_StripsAppAndComment(t *testing.T) {
	cases := []struct {
		name   string
		marker byte
	}{
		{"APP1 EXIF", 0xE1},
		{"APP2 ICC", 0xE2},
		{"APP13 Photoshop", 0xED},
		{"APP14 Adobe", 0xEE},
		{"COM", 0xFE},
	}

	original := buildJPEG(16, 16)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bomb := bytes.Repeat([]byte{0x42}, 32*1024)
			src := insertJPEGSegment(original, c.marker, bomb)

			out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
			if !ok {
				t.Fatal("Sanitize() = rejected, want stripped and accepted")
			}
			if len(out) >= len(src) {
				t.Fatalf("output len=%d, want smaller than input len=%d", len(out), len(src))
			}
			assertSameJPEGSize(t, original, out)
		})
	}
}

// TestSanitizeJPEG_KeepsAGenuineAdobeSegment covers APP14 in its one
// real shape, the fixed 12-byte "Adobe" block that tells a decoder how
// the colour channels are encoded: dropping it decodes an RGB JPEG with
// the wrong colours and makes a CMYK one undecodable. Anything larger
// under that marker is still stripped (see above).
func TestSanitizeJPEG_KeepsAGenuineAdobeSegment(t *testing.T) {
	adobe := []byte{'A', 'd', 'o', 'b', 'e', 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x01}
	src := insertJPEGSegment(buildJPEG(16, 16), 0xEE, adobe)

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Contains(out, append([]byte{0xFF, 0xEE, 0x00, 0x0E}, adobe...)) {
		t.Error("sanitized output dropped the Adobe APP14 segment")
	}
}

// TestSanitizeJPEG_KeepsAPP0 covers the one APPn segment sanitizeJPEG
// must not drop: plain JFIF, which carries no metadata a decoder
// treats as text or a compressed stream.
func TestSanitizeJPEG_KeepsAPP0(t *testing.T) {
	src := insertJPEGSegment(buildJPEG(16, 16), 0xE0, []byte("JFIF\x01\x01\x00\x00\x01\x00\x01\x00\x00"))

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Contains(out, []byte{0xFF, 0xE0}) {
		t.Fatal("output dropped APP0, which this test added deliberately")
	}
}

// TestSanitizeJPEG_RejectsTruncatedSegment covers a segment whose
// declared length runs past the end of the file.
func TestSanitizeJPEG_RejectsTruncatedSegment(t *testing.T) {
	src := insertJPEGSegment(buildJPEG(16, 16), 0xE1, bytes.Repeat([]byte{1}, 100))
	truncated := src[:len(src)-80]

	if _, _, _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated file, want rejected")
	}
}

// TestSanitizeJPEG_RejectsOversizedDimensions covers a start-of-frame
// segment declaring a huge width and height.
func TestSanitizeJPEG_RejectsOversizedDimensions(t *testing.T) {
	src := buildJPEG(8, 8)
	sof := bytes.Index(src, []byte{0xFF, 0xC0})
	if sof < 0 {
		t.Fatal("test JPEG has no baseline SOF0 marker")
	}

	// SOF0 payload: precision(1) height(2) width(2) ...; set both to
	// 60000, far past any sane photo and past FullImage's own cap.
	src[sof+5], src[sof+6] = 0xEA, 0x60
	src[sof+7], src[sof+8] = 0xEA, 0x60

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 60000x60000 header, want rejected")
	}
}

// TestSanitizeJPEG_RejectsMissingSOF covers a scan header reached
// before any start-of-frame segment ever set the dimensions.
func TestSanitizeJPEG_RejectsMissingSOF(t *testing.T) {
	src := []byte{0xFF, 0xD8, 0xFF, 0xDA, 0x00, 0x02, 0xFF, 0xD9}

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a file with no SOF, want rejected")
	}
}

// TestSanitizeJPEG_RejectsShortSOF covers a start-of-frame segment
// too short to carry the height and width fields jpegDimensions reads.
func TestSanitizeJPEG_RejectsShortSOF(t *testing.T) {
	src := []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x03, 0x08, 0xFF, 0xD9}

	if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated SOF, want rejected")
	}
}

// TestSanitizeJPEG_RejectsShortMarker covers a file cut off right
// after the SOI, with no complete marker to read next.
func TestSanitizeJPEG_RejectsShortMarker(t *testing.T) {
	if _, _, _, ok := imagesafe.Sanitize([]byte{0xFF, 0xD8, 0xFF}, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a file with only SOI, want rejected")
	}
}

// TestSanitizeJPEG_CapsRepeatedScans covers the exact gap a security
// review found: copyJPEGRemainder copied every later start-of-scan
// segment through unfiltered, so a progressive JPEG could repeat a
// tiny scan about a million times inside a file well under this
// package's own size cap, each repeat walking the decoder across
// every block of the image again - minutes to hours of CPU inside
// omarchy-shell, with no click needed. An ordinary progressive encode
// never comes close to this many scans.
func TestSanitizeJPEG_CapsRepeatedScans(t *testing.T) {
	original := buildJPEG(64, 64)

	t.Run("an ordinary number of scans is accepted", func(t *testing.T) {
		// The header's own first SOS, plus 9 more: 10 total, well
		// within what a real progressive encode produces.
		src := insertFakeMarkers(original, 0xDA, 9)

		if _, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); !ok {
			t.Error("Sanitize() = rejected for an ordinary progressive scan count, want accepted")
		}
	})

	t.Run("a far larger number of scans is refused quickly", func(t *testing.T) {
		src := insertFakeMarkers(original, 0xDA, 2_000_000)

		start := time.Now()
		_, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Sanitize() took %v for a file repeating its scan two million times, want well under a second", elapsed)
		}
		if ok {
			t.Error("Sanitize() = accepted for a file repeating its scan two million times, want rejected")
		}
	})
}

// TestSanitizeJPEG_CapsRepeatedTableSegments covers the same kind of
// unfiltered repetition for DHT and DQT segments: cheaper per
// repetition than a scan, but still unbounded before this fix, and
// just as far from anything a real encoder produces.
func TestSanitizeJPEG_CapsRepeatedTableSegments(t *testing.T) {
	original := buildJPEG(64, 64)

	cases := []struct {
		name   string
		marker byte
	}{
		{"DHT", 0xC4},
		{"DQT", 0xDB},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ordinary := insertFakeMarkers(original, c.marker, 20)
			if _, _, _, ok := imagesafe.Sanitize(ordinary, imagesafe.FullImage); !ok {
				t.Errorf("Sanitize() = rejected for an ordinary number of %s segments, want accepted", c.name)
			}

			excessive := insertFakeMarkers(original, c.marker, 100_000)
			if _, _, _, ok := imagesafe.Sanitize(excessive, imagesafe.FullImage); ok {
				t.Errorf("Sanitize() = accepted for %s repeated far beyond any real encoder's use, want rejected", c.name)
			}
		})
	}
}

// TestSanitizeJPEG_CountsMarkersPastFillBytes covers the exact bypass
// a follow-up security review found: copyJPEGRemainder classified the
// byte right after the first 0xFF as the marker code, so a marker
// preceded by one or more extra 0xFF fill bytes - FF FF DA rather than
// a plain FF DA - was never counted at all, letting a repeated scan,
// DHT or DQT segment back past jpegMaxScans and
// jpegMaxRepeatedTableSegments the same way TestSanitizeJPEG_CapsRepeatedScans
// and TestSanitizeJPEG_CapsRepeatedTableSegments already cover without
// the extra fill byte.
func TestSanitizeJPEG_CountsMarkersPastFillBytes(t *testing.T) {
	original := buildJPEG(64, 64)

	cases := []struct {
		name   string
		marker byte
	}{
		{"SOS", 0xDA},
		{"DHT", 0xC4},
		{"DQT", 0xDB},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := insertFakeMarkersWithFill(original, c.marker, 1, 2_000_000)

			start := time.Now()
			_, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("Sanitize() took %v for a fill-padded marker repeated two million times, want well under a second", elapsed)
			}
			if ok {
				t.Errorf("Sanitize() = accepted for a %s marker hidden behind one fill byte and repeated two million times, want rejected", c.name)
			}
		})
	}
}

// TestSanitizeJPEG_CollapsesOrdinaryFillBytes covers the case any real
// encoder actually produces: a marker preceded by a small, ordinary
// run of fill bytes must still be accepted, with the run collapsed in
// the output to the marker's own ordinary leading 0xFF rather than
// kept at full length or rejected outright. The spliced-in markers
// have no real scan data behind them (see insertFakeMarkersWithFill),
// the same as TestSanitizeJPEG_CapsRepeatedScans, so this checks the
// output's bytes directly rather than attempting a full re-decode.
func TestSanitizeJPEG_CollapsesOrdinaryFillBytes(t *testing.T) {
	original := buildJPEG(64, 64)
	src := insertFakeMarkersWithFill(original, 0xDA, 3, 2)

	out, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected for an ordinary fill-byte run before a marker, want accepted")
	}

	uncollapsed := append(bytes.Repeat([]byte{0xFF}, 4), 0xDA)
	if bytes.Contains(out, uncollapsed) {
		t.Error("output kept the fill-byte run uncollapsed before the marker, want it collapsed")
	}
	if !bytes.Contains(out, []byte{0xFF, 0xDA}) {
		t.Error("output dropped the marker along with its fill bytes, want it kept and collapsed")
	}
}

// TestSanitizeJPEG_CapsTotalFillBytes covers a single, huge run of
// fill bytes that never resolves into enough markers to trip the scan
// or table caps on its own: copyJPEGRemainder must still refuse once
// the total number of fill bytes it has skipped crosses
// jpegMaxFillBytes.
func TestSanitizeJPEG_CapsTotalFillBytes(t *testing.T) {
	original := buildJPEG(64, 64)
	// One marker, preceded by a single fill run far past
	// jpegMaxFillBytes: not enough scans or tables on their own to
	// trip those caps, so only the fill-byte cap can catch this.
	src := insertFakeMarkersWithFill(original, 0xDA, 200_000, 1)

	start := time.Now()
	_, _, _, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Sanitize() took %v for a single oversized fill run, want well under a second", elapsed)
	}
	if ok {
		t.Error("Sanitize() = accepted for a fill run far past jpegMaxFillBytes, want rejected")
	}
}

// assertSameJPEGSize decodes both JPEGs and fails t if their
// dimensions differ: lossy re-encoding inside the stdlib encoder used
// to build the fixture means pixel-for-pixel equality is not
// guaranteed, but the sanitizer touches no pixel data at all, so the
// decoded size is the one thing that must always match exactly.
func assertSameJPEGSize(t *testing.T, want []byte, got []byte) {
	t.Helper()

	wantImg, err := jpeg.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	gotImg, err := jpeg.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode sanitized: %v", err)
	}
	if wantImg.Bounds() != gotImg.Bounds() {
		t.Fatalf("sanitized bounds=%v, want=%v", gotImg.Bounds(), wantImg.Bounds())
	}
}

// FuzzSanitizeJPEG feeds arbitrary bytes, and small mutations of a
// real JPEG, to make sure nothing ever panics and nothing is ever
// returned as "ok" without decoding.
func FuzzSanitizeJPEG(f *testing.F) {
	seed := buildJPEG(16, 16)
	f.Add(seed)
	f.Add(insertJPEGSegment(seed, 0xE1, bytes.Repeat([]byte{1}, 2048)))
	f.Add([]byte{})
	f.Add(seed[:len(seed)-4])
	f.Add(insertFakeMarkers(seed, 0xDA, 500))                // scans repeated past jpegMaxScans
	f.Add(insertFakeMarkers(seed, 0xC4, 5_000))              // DHT repeated past jpegMaxRepeatedTableSegments
	f.Add(insertFakeMarkersWithFill(seed, 0xDA, 1, 500))     // the fill-byte bypass: scans hidden behind one extra 0xFF each
	f.Add(insertFakeMarkersWithFill(seed, 0xC4, 1, 5_000))   // the same bypass for a repeated DHT segment
	f.Add(insertFakeMarkersWithFill(seed, 0xDA, 100_000, 1)) // a single fill run past jpegMaxFillBytes on its own

	f.Fuzz(func(t *testing.T, data []byte) {
		out, _, _, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
		if !ok {
			if out != nil {
				t.Fatal("rejected input returned non-nil output")
			}
			return
		}
		// sanitizeJPEG's own confirmation decode is a cheap
		// DecodeConfig, not a full decode (see finishJPEG): a
		// corrupt scan JPEG has no checksum to catch it the way a
		// PNG chunk's CRC does, so this only re-checks what Sanitize
		// itself promises, not full decodability.
		if _, err := jpeg.DecodeConfig(bytes.NewReader(out)); err != nil {
			t.Fatalf("accepted output's header no longer parses: %v", err)
		}
	})
}
