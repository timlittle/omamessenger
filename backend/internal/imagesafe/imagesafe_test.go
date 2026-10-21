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
			if _, ok := imagesafe.Sanitize(data, imagesafe.FullImage); ok {
				t.Fatalf("Sanitize(%q) = ok, want rejected", name)
			}
		})
	}
}

// TestSanitize_RejectsOverInputLenCap covers data that is itself
// larger than a profile ever allows, before any format parsing runs.
func TestSanitize_RejectsOverInputLenCap(t *testing.T) {
	data := buildPNG(4, 4)
	profile := imagesafe.Profile{MaxSide: 8, MaxPixels: 64, MaxInputLen: int64(len(data) - 1)}

	if _, ok := imagesafe.Sanitize(data, profile); ok {
		t.Fatal("Sanitize() = ok, want rejected for data over MaxInputLen")
	}
}
