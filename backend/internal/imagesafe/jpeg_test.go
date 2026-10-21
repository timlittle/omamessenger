package imagesafe_test

import (
	"bytes"
	"image/jpeg"
	"testing"

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

			out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
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

// TestSanitizeJPEG_KeepsAPP0 covers the one APPn segment sanitizeJPEG
// must not drop: plain JFIF, which carries no metadata a decoder
// treats as text or a compressed stream.
func TestSanitizeJPEG_KeepsAPP0(t *testing.T) {
	src := insertJPEGSegment(buildJPEG(16, 16), 0xE0, []byte("JFIF\x01\x01\x00\x00\x01\x00\x01\x00\x00"))

	out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
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

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
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

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 60000x60000 header, want rejected")
	}
}

// TestSanitizeJPEG_RejectsMissingSOF covers a scan header reached
// before any start-of-frame segment ever set the dimensions.
func TestSanitizeJPEG_RejectsMissingSOF(t *testing.T) {
	src := []byte{0xFF, 0xD8, 0xFF, 0xDA, 0x00, 0x02, 0xFF, 0xD9}

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a file with no SOF, want rejected")
	}
}

// TestSanitizeJPEG_RejectsShortSOF covers a start-of-frame segment
// too short to carry the height and width fields jpegDimensions reads.
func TestSanitizeJPEG_RejectsShortSOF(t *testing.T) {
	src := []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x03, 0x08, 0xFF, 0xD9}

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated SOF, want rejected")
	}
}

// TestSanitizeJPEG_RejectsShortMarker covers a file cut off right
// after the SOI, with no complete marker to read next.
func TestSanitizeJPEG_RejectsShortMarker(t *testing.T) {
	if _, ok := imagesafe.Sanitize([]byte{0xFF, 0xD8, 0xFF}, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a file with only SOI, want rejected")
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

	f.Fuzz(func(t *testing.T, data []byte) {
		out, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
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
