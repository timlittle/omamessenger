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
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestPlaceholderGIF_PassesTheHelperSOwnGIFCheck guards
// tests/qml/Flows's own end-to-end check that a scripted real GIF
// document animates: if this fixture's bytes ever stopped being a
// genuine, sanitizable GIF image, app.DecodeAsForMedia's
// connector.SanitizeIfGIF call (the same one a real sender's download
// goes through) would start reporting false, and that test would time
// out waiting for a frame that never plays, far from where the real
// cause actually is.
func TestPlaceholderGIF_PassesTheHelperSOwnGIFCheck(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "run.gif")
	if err := os.WriteFile(path, placeholderGIF(), 0o600); err != nil {
		t.Fatal(err)
	}

	isGIF, err := connector.SanitizeIfGIF(path)
	if err != nil || !isGIF {
		t.Fatalf("SanitizeIfGIF() = %t, %v, want true, nil", isGIF, err)
	}
}

// TestPlaceholderGIF_StaysWithinTheAnimatedGIFProfile confirms the
// scene is sized and framed so that it actually shows something
// recognisable (not the old 64x64 dot) while still comfortably inside
// imagesafe.AnimatedGIF's own per-frame and total-pixel bounds, so a
// future change to the scene cannot accidentally make it too slow to
// decode or too big to pass app.DecodeAsForMedia's own check.
func TestPlaceholderGIF_StaysWithinTheAnimatedGIFProfile(t *testing.T) {
	t.Parallel()

	decoded, err := gif.DecodeAll(bytes.NewReader(placeholderGIF()))
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded.Image) != gifFrames {
		t.Errorf("placeholderGIF() has %d frames, want %d", len(decoded.Image), gifFrames)
	}

	bounds := decoded.Image[0].Bounds()
	if bounds.Dx() != gifWidth || bounds.Dy() != gifHeight {
		t.Errorf("placeholderGIF() frame size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), gifWidth, gifHeight)
	}

	framePixels := int64(gifWidth) * int64(gifHeight)
	if framePixels > imagesafe.AnimatedGIF.MaxPixels {
		t.Errorf("placeholderGIF() frame is %d pixels, over imagesafe.AnimatedGIF's own %d cap", framePixels, imagesafe.AnimatedGIF.MaxPixels)
	}

	totalPixels := framePixels * int64(gifFrames)
	if totalPixels > imagesafe.AnimatedGIF.MaxTotalPixels {
		t.Errorf("placeholderGIF() totals %d pixels, over imagesafe.AnimatedGIF's own %d cap", totalPixels, imagesafe.AnimatedGIF.MaxTotalPixels)
	}
}

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
