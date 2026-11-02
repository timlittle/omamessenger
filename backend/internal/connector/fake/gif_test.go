// gif_test.go is an internal test (package fake, not fake_test) because
// placeholderGIF is unexported and there is no public API that returns
// the fake connector's own GIF bytes: production code never needs them
// outside this package.
package fake

import (
	"bytes"
	"encoding/base64"
	"image/gif"
	"image/jpeg"
	"testing"
)

// TestGifThumb_IsAValidJPEGStillAtTheSameSize confirms the still
// preview an unplayed GIF bubble shows (see domain.Media.Thumb) is a
// genuine, base64-encoded JPEG at the animation's own size, so the
// bubble's Image element has real pixels to draw instead of an empty
// box before anyone has clicked to play it.
func TestGifThumb_IsAValidJPEGStillAtTheSameSize(t *testing.T) {
	t.Parallel()

	raw, err := base64.StdEncoding.DecodeString(gifThumb())
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != gifWidth || cfg.Height != gifHeight {
		t.Errorf("gifThumb() still = %dx%d, want %dx%d", cfg.Width, cfg.Height, gifWidth, gifHeight)
	}
}

// TestPlaceholderGIF_IsARichMovingScene guards against the GIF sliding
// back into a few flat bands that read as a rendering glitch in a demo
// recording: the frames must use a broad palette, for smooth gradients,
// and consecutive frames must differ, so it visibly animates.
func TestPlaceholderGIF_IsARichMovingScene(t *testing.T) {
	t.Parallel()

	decoded, err := gif.DecodeAll(bytes.NewReader(placeholderGIF()))
	if err != nil {
		t.Fatal(err)
	}

	if n := len(decoded.Image[0].Palette); n < 128 {
		t.Errorf("placeholderGIF() palette has %d colours, want at least 128", n)
	}
	if bytes.Equal(decoded.Image[0].Pix, decoded.Image[1].Pix) {
		t.Error("placeholderGIF() frames 0 and 1 are identical, want the water to move")
	}
}
