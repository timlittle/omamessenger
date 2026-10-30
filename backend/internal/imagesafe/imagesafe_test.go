package imagesafe_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSanitize_RejectsUnknownFormat covers every format this package
// does not recognize: a sender's file is shown as a file, never
// forwarded to an Image component, when its own bytes are not one of
// the formats this package knows how to check.
func TestSanitize_RejectsUnknownFormat(t *testing.T) {
	cases := map[string][]byte{
		"empty":           {},
		"plain text":      []byte("not an image at all"),
		"bmp magic":       {'B', 'M', 0, 0, 0, 0},
		"truncated magic": {0x89, 'P', 'N'},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := imagesafe.Sanitize(data, imagesafe.FullImage); ok {
				t.Fatalf("Sanitize(%q) = ok, want rejected", name)
			}
		})
	}
}

// TestLooksLikeImage_MatchesOnlyRecognizedFormats covers every format
// Sanitize recognizes, plus the same unrecognized cases
// TestSanitize_RejectsUnknownFormat does: LooksLikeImage must agree
// with Sanitize about which is which, even though it runs none of
// Sanitize's own structural or size checks.
func TestLooksLikeImage_MatchesOnlyRecognizedFormats(t *testing.T) {
	cases := map[string]struct {
		data []byte
		want bool
	}{
		"png":             {buildPNG(2, 2), true},
		"jpeg":            {buildJPEG(2, 2), true},
		"gif":             {buildGIF(2, 2, 1), true},
		"webp":            {buildWebPSimple(2, 2), true},
		"empty":           {[]byte{}, false},
		"plain text":      {[]byte("not an image at all"), false},
		"bmp magic":       {[]byte{'B', 'M', 0, 0, 0, 0}, false},
		"truncated magic": {[]byte{0x89, 'P', 'N'}, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := imagesafe.LooksLikeImage(tc.data); got != tc.want {
				t.Errorf("LooksLikeImage(%s) = %t, want %t", name, got, tc.want)
			}
		})
	}
}

// TestSanitize_RejectsOverInputLenCap covers data that is itself
// larger than a profile ever allows, before any format parsing runs.
func TestSanitize_RejectsOverInputLenCap(t *testing.T) {
	data := buildPNG(4, 4)
	profile := imagesafe.Profile{MaxSide: 8, MaxPixels: 64, MaxInputLen: int64(len(data) - 1)}

	if _, _, _, ok := imagesafe.Sanitize(data, profile); ok {
		t.Fatal("Sanitize() = ok, want rejected for data over MaxInputLen")
	}
}

// TestSanitize_RejectsExtremeAspectRatio covers the decode-bomb shape
// a very thin, very long image makes: Qt's own sourceSize scaling
// covers a box by scaling to an image's longer side, so even a tiny,
// well-formed thumbnail that is 1 pixel wide and 1024 pixels tall can
// make the UI decode on the order of 100 megapixels once it is scaled
// to fit a normal message bubble (see docs/decisions.md). Every real
// profile's own MaxAspectRatio must catch this shape even though
// every other cap - side, pixel count and input length - still
// passes it.
func TestSanitize_RejectsExtremeAspectRatio(t *testing.T) {
	cases := map[string]struct {
		data    []byte
		profile imagesafe.Profile
	}{
		"png thumbnail":  {buildPNG(1, 1024), imagesafe.Thumbnail},
		"jpeg thumbnail": {buildJPEG(4, 1024), imagesafe.Thumbnail},
		"gif thumbnail":  {buildGIF(1, 1024, 1), imagesafe.Thumbnail},
		"webp thumbnail": {buildWebPSimple(1, 1024), imagesafe.Thumbnail},
		"png full image": {buildPNG(1, 8192), imagesafe.FullImage},
		"png animated":   {buildPNG(1, 2048), imagesafe.AnimatedGIF},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := imagesafe.Sanitize(tc.data, tc.profile); ok {
				t.Fatalf("Sanitize(%s) = ok, want rejected for an extreme aspect ratio", name)
			}
		})
	}
}

// TestSanitize_AcceptsRealisticAspectRatios covers the aspect ratios
// a genuine photo actually has - a tall phone screenshot and a wide
// panorama - so MaxAspectRatio's own cap never rejects real content,
// only the decode-bomb shape above.
func TestSanitize_AcceptsRealisticAspectRatios(t *testing.T) {
	cases := map[string]struct {
		data    []byte
		profile imagesafe.Profile
	}{
		"tall phone screenshot, about 1:2.2": {buildPNG(1080, 2400), imagesafe.FullImage},
		"wide panorama, about 10:1":          {buildPNG(2000, 200), imagesafe.FullImage},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := imagesafe.Sanitize(tc.data, tc.profile); !ok {
				t.Fatalf("Sanitize(%s) = rejected, want accepted as a realistic photo", name)
			}
		})
	}
}
