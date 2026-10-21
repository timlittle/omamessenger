package imagesafe_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSanitizePNG_StripsCompressedMetadata covers the reported attack:
// a PNG carrying a large zTXt, iTXt or iCCP chunk, each a zlib stream
// Qt's own reader still inflates even though the UI only ever shows
// the pixels. The sanitized output must drop the chunk but still
// decode to the same image.
func TestSanitizePNG_StripsCompressedMetadata(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{"zTXt", "zTXt"},
		{"iTXt", "iTXt"},
		{"iCCP", "iCCP"},
		{"tEXt", "tEXt"},
		{"eXIf", "eXIf"},
	}

	original := buildPNG(8, 8)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bomb := make([]byte, 64*1024)
			src := insertPNGChunk(original, c.kind, bomb)

			out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
			if !ok {
				t.Fatalf("Sanitize() = rejected, want stripped and accepted")
			}
			if bytes.Contains(out, []byte(c.kind)) {
				t.Fatalf("output still contains a %s chunk", c.kind)
			}
			if len(out) >= len(src) {
				t.Fatalf("output len=%d, want smaller than input len=%d", len(out), len(src))
			}
			assertSamePixels(t, original, out)
		})
	}
}

// TestSanitizePNG_RejectsOversizedDimensions covers the reported pixel
// bomb: a header declaring a huge width and height, without any pixel
// data anywhere near that large.
func TestSanitizePNG_RejectsOversizedDimensions(t *testing.T) {
	src := setPNGDimensions(buildPNG(4, 4), 100000, 100000)

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a 100000x100000 header, want rejected")
	}
}

// TestSanitizePNG_RejectsBadCRC covers a chunk whose CRC does not
// match its own type and data: never trusted, whatever it claims to be.
func TestSanitizePNG_RejectsBadCRC(t *testing.T) {
	src := insertPNGChunk(buildPNG(8, 8), "tEXt", []byte("hello"))

	// Flip a bit inside the chunk's CRC, four bytes before where the
	// next chunk starts.
	crcPos := len(src) - lengthOfTrailingChunks(src) - 4
	src[crcPos] ^= 0xFF

	if _, ok := imagesafe.Sanitize(src, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a chunk with a bad CRC, want rejected")
	}
}

// TestSanitizePNG_RejectsTruncatedChunk covers a chunk whose declared
// length runs past the end of the file.
func TestSanitizePNG_RejectsTruncatedChunk(t *testing.T) {
	src := buildPNG(8, 8)
	truncated := src[:len(src)-10]

	if _, ok := imagesafe.Sanitize(truncated, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a truncated file, want rejected")
	}
}

// TestSanitizePNG_KeepsRenderingChunks covers the chunks sanitizePNG
// must not drop: gAMA, pHYs and a palette all still need to reach the
// decoder for a paletted or gamma-corrected image to render correctly.
func TestSanitizePNG_KeepsRenderingChunks(t *testing.T) {
	src := insertPNGChunk(buildPNG(8, 8), "gAMA", []byte{0, 0, 0x9a, 0x9a})

	out, ok := imagesafe.Sanitize(src, imagesafe.FullImage)
	if !ok {
		t.Fatal("Sanitize() = rejected, want accepted")
	}
	if !bytes.Contains(out, []byte("gAMA")) {
		t.Fatal("output dropped gAMA, which rendering needs")
	}
}

// TestSanitizePNG_RejectsMissingIHDR covers a signature not
// immediately followed by an IHDR chunk, which pngDimensions treats
// as not a PNG at all, whatever its magic bytes claimed.
func TestSanitizePNG_RejectsMissingIHDR(t *testing.T) {
	src := buildPNG(8, 8)
	var out bytes.Buffer
	out.Write(src[:8]) // signature only
	writeChunk(&out, "tEXt", []byte("no IHDR here"))
	out.Write(src[8:])

	if _, ok := imagesafe.Sanitize(out.Bytes(), imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a file with no IHDR first, want rejected")
	}
}

// TestSanitizePNG_RejectsHugeChunkLength covers a declared chunk
// length too large to even fit in this package's own int arithmetic,
// not just too large for the remaining buffer.
func TestSanitizePNG_RejectsHugeChunkLength(t *testing.T) {
	src := buildPNG(8, 8)
	out := append([]byte{}, src...)
	binary.BigEndian.PutUint32(out[8:12], 0xFFFFFFFF)

	if _, ok := imagesafe.Sanitize(out, imagesafe.FullImage); ok {
		t.Fatal("Sanitize() = ok for a chunk declaring a 4-gigabyte length, want rejected")
	}
}

// assertSamePixels decodes both PNGs and fails t if they differ.
func assertSamePixels(t *testing.T, want []byte, got []byte) {
	t.Helper()

	wantImg, err := png.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	gotImg, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode sanitized: %v", err)
	}
	if !imagesMatch(wantImg, gotImg) {
		t.Fatal("sanitized image decodes to different pixels than the original")
	}
}

// imagesMatch reports whether two images have the same bounds and pixels.
func imagesMatch(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

// lengthOfTrailingChunks is the byte length of every chunk after the
// one insertPNGChunk just added: the rest of a PNG built by buildPNG,
// which is always IDAT once (or more) then IEND.
func lengthOfTrailingChunks(src []byte) int {
	pos := 8 + 8 + 13 + 4 // past signature and IHDR
	// skip the just-inserted chunk
	length := binary.BigEndian.Uint32(src[pos : pos+4])
	pos += 8 + int(length) + 4
	return len(src) - pos
}

// FuzzSanitizePNG feeds arbitrary bytes, and small mutations of a real
// PNG, to make sure nothing ever panics and nothing is ever returned
// as "ok" without decoding.
func FuzzSanitizePNG(f *testing.F) {
	seed := buildPNG(8, 8)
	f.Add(seed)
	f.Add(insertPNGChunk(seed, "zTXt", bytes.Repeat([]byte{1}, 2048)))
	f.Add(setPNGDimensions(seed, 50000, 50000))
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
		if _, err := png.Decode(bytes.NewReader(out)); err != nil {
			t.Fatalf("accepted output does not decode: %v", err)
		}
	})
}
