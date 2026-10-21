package whatsapp

// imagehelpers_test.go holds the real-image fixtures this package's
// tests use to prove a thumbnail or a downloaded photo, sticker or
// GIF actually routes through imagesafe, split out of helpers_test.go
// once that file grew past the line limit (see docs/decisions.md's
// own note on splitting a shared test-helpers file by what it holds).

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// tinyJPEG returns a tiny, real JPEG, for a test that needs bytes
// imagesafe actually accepts rather than an arbitrary placeholder: a
// sanitizer that only ever saw its own hand-rolled fixtures would
// prove nothing about whether a real image still gets through.
func tinyJPEG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 100, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode tiny JPEG fixture: %v", err)
	}

	return buf.Bytes()
}

// jpegDecodeConfig reports whether data decodes as a JPEG, for a test
// that only needs to know the sanitized bytes are still a real image,
// not its exact pixels.
func jpegDecodeConfig(data []byte) (image.Config, error) {
	return jpeg.DecodeConfig(bytes.NewReader(data))
}
