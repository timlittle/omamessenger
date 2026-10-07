package app_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestPasteImage_CopiesThePreferredImageType(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	var imgBytes bytes.Buffer
	if err := encodePNG(&imgBytes, 4, 3); err != nil {
		t.Fatal(err)
	}

	f.clipboard.types = []string{"text/plain", "image/png", "image/jpeg"}
	f.clipboard.data = map[string][]byte{"image/png": imgBytes.Bytes()}

	img, err := f.commands.PasteImage(t.Context())
	if err != nil {
		t.Fatalf("PasteImage() error = %v", err)
	}

	if img.Kind != domain.MediaPhoto || img.Width != 4 || img.Height != 3 {
		t.Errorf("PasteImage() = %+v, want a 4x3 photo", img)
	}

	got, err := os.ReadFile(img.Path)
	if err != nil || !bytes.Equal(got, imgBytes.Bytes()) {
		t.Errorf("stored clipboard image: %v, equal=%t", err, bytes.Equal(got, imgBytes.Bytes()))
	}
}

func TestPasteImage_RejectsAClipboardWithNoImage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.clipboard.types = []string{"text/plain"}

	if _, err := f.commands.PasteImage(t.Context()); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("PasteImage(no image) = %v, want ErrInvalidInput", err)
	}
}

func TestPasteImage_ReportsAClipboardFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.clipboard.typesErr = errors.New("wl-paste not found")

	if _, err := f.commands.PasteImage(t.Context()); err == nil {
		t.Error("PasteImage() with a failing clipboard = nil error, want one")
	}
}

// encodePNG writes a solid image of the given size as a PNG to w.
func encodePNG(w *bytes.Buffer, width, height int) error {
	return png.Encode(w, image.NewRGBA(image.Rect(0, 0, width, height)))
}
