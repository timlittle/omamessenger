package app_test

// imagehelpers_test.go holds assertSamePixels, split out of
// helpers_test.go to keep that file under the project's own file
// length limit: Reencode's own re-encoding means a display copy's
// bytes are never byte-for-byte identical to the original it was
// built from, only its pixels (see docs/decisions.md), so several
// tests across this package need this one check.

import (
	"bytes"
	"image"
	"testing"
)

// assertSamePixels fails t unless want and got decode (through
// whichever format each actually is - app's own production code
// already registers PNG, JPEG and GIF, see attachment.go) to images
// with the same bounds and pixels: the property Reencode guarantees,
// even once a display copy's own bytes differ from the original -
// a different container format, compression or metadata entirely -
// which rules out a plain bytes.Equal between the two (see
// connector.ReencodeImageFile and docs/decisions.md).
func assertSamePixels(t *testing.T, want, got []byte) {
	t.Helper()

	wantImg, _, err := image.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("decode want: %v", err)
	}
	gotImg, _, err := image.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode got: %v", err)
	}

	wb, gb := wantImg.Bounds(), gotImg.Bounds()
	if wb.Dx() != gb.Dx() || wb.Dy() != gb.Dy() {
		t.Fatalf("bounds = %dx%d, want %dx%d", gb.Dx(), gb.Dy(), wb.Dx(), wb.Dy())
	}
	for y := 0; y < wb.Dy(); y++ {
		for x := 0; x < wb.Dx(); x++ {
			if wantImg.At(wb.Min.X+x, wb.Min.Y+y) != gotImg.At(gb.Min.X+x, gb.Min.Y+y) {
				t.Fatalf("pixel (%d,%d) differs between want and got", x, y)
			}
		}
	}
}
