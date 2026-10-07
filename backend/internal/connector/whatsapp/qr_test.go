package whatsapp

// qrPNG is unexported and has no public way to be driven, so this test
// calls it directly rather than through Connector's exported API.

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
)

func TestQRPNG_DrawsADecodableImage(t *testing.T) {
	t.Parallel()

	encoded, err := qrPNG("1@abc,def,ghi==,0")
	if err != nil {
		t.Fatal(err)
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("not valid base64: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a valid PNG: %v", err)
	}

	if b := img.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("image bounds = %v, want a non-empty square", b)
	}
}

func TestQRPNG_RejectsDataTooLargeToEncode(t *testing.T) {
	t.Parallel()

	if _, err := qrPNG(strings.Repeat("x", 10_000)); err == nil {
		t.Error("qrPNG = nil error, want one for oversized data")
	}
}
