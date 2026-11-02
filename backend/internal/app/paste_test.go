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

// TestPasteImage_RejectsAnUnsafeImage confirms a pasted image is run
// through the same safety check a downloaded photo must pass: the
// clipboard belongs to whichever other program last put something on
// it, so bytes that are not a real, safely-sized image are refused
// rather than written to the outgoing media area unchecked.
func TestPasteImage_RejectsAnUnsafeImage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.clipboard.types = []string{"image/png"}
	f.clipboard.data = map[string][]byte{"image/png": []byte("not a real image")}

	if _, err := f.commands.PasteImage(t.Context()); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("PasteImage(unsafe image) = %v, want ErrInvalidInput", err)
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

// TestPasteText_ReturnsTheClipboardsText confirms PasteText hands back
// whatever ReadText found, through the same bounded wl-paste path
// PasteImage already uses for an image.
func TestPasteText_ReturnsTheClipboardsText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.clipboard.data = map[string][]byte{"text/plain": []byte("hello there")}

	got, err := f.commands.PasteText(t.Context())
	if err != nil {
		t.Fatalf("PasteText() error = %v", err)
	}
	if got != "hello there" {
		t.Errorf("PasteText() = %q, want %q", got, "hello there")
	}
}

// TestPasteText_RejectsAnEmptyClipboard confirms an empty text/plain
// offer is reported as invalid input rather than silently inserting
// nothing, the same as PasteImage does when the clipboard holds no
// image.
func TestPasteText_RejectsAnEmptyClipboard(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.PasteText(t.Context()); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("PasteText(empty clipboard) = %v, want ErrInvalidInput", err)
	}
}

// TestPasteText_ReportsAClipboardFailure confirms a failing
// clipboard.ReadText (the helper's own bounded read refused a clipboard
// that was too large or too slow) is reported as an error rather than
// treated as empty.
func TestPasteText_ReportsAClipboardFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.clipboard.readErr = errors.New("clipboard: paste exceeds the size limit")

	if _, err := f.commands.PasteText(t.Context()); err == nil {
		t.Error("PasteText() with a failing clipboard = nil error, want one")
	}
}

// encodePNG writes a solid image of the given size as a PNG to w.
func encodePNG(w *bytes.Buffer, width, height int) error {
	return png.Encode(w, image.NewRGBA(image.Rect(0, 0, width, height)))
}
