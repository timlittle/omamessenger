package signal

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"testing"
)

// TestQRPNG_DecodesAsPNG checks that the drawn code is a valid PNG a
// browser or the UI's own Image element can decode.
func TestQRPNG_DecodesAsPNG(t *testing.T) {
	t.Parallel()

	encoded, err := qrPNG("sgnl://linkdevice?uuid=test&pub_key=abc")
	if err != nil {
		t.Fatalf("qrPNG() error = %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}

	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Error("decoded image has no size")
	}
}

// TestQRPNG_RejectsTooMuchData checks that data too large for a QR
// code reports an error rather than panicking.
func TestQRPNG_RejectsTooMuchData(t *testing.T) {
	t.Parallel()

	huge := make([]byte, 10_000)
	if _, err := qrPNG(string(huge)); err == nil {
		t.Error("qrPNG() succeeded with far more data than a QR code holds")
	}
}
