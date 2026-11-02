package connector_test

// media_test.go checks ReencodeImageFile and SaveMedia directly.
// ReencodeImageFile is the one place a photo, sticker or GIF's pixels
// are ever decoded for display: app.Commands calls it once to fill a
// display copy's own cache slot, never again on every time that slot
// is served, since re-encoding is a lossy, one-way rewrite (see
// docs/decisions.md).

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestReencodeImageFile_WritesTheReencodedResult covers the basic
// round trip: src is read and left exactly as it was, and dst
// receives imagesafe.Reencode's own result, never src's own bytes.
func TestReencodeImageFile_WritesTheReencodedResult(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "original.bin")
	original := onePixelPNG()
	if err := os.WriteFile(src, original, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "view.jpg")

	format, width, height, err := connector.ReencodeImageFile(src, dst, imagesafe.Thumbnail)
	if err != nil {
		t.Fatalf("ReencodeImageFile() = %v, want nil", err)
	}
	if format != imagesafe.FormatJPEG {
		t.Errorf("format = %v, want FormatJPEG for an opaque 1x1 PNG", format)
	}
	wantDims(t, width, height, 1, 1)

	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if bytes.Equal(out, original) {
		t.Error("destination holds the original bytes unchanged, want a fresh re-encode")
	}

	stillThere, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(stillThere, original) {
		t.Error("source file was modified, want it left exactly as it was")
	}

	info, err := os.Stat(dst)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("destination mode = %v, %v, want 0600", info, err)
	}
}

// TestReencodeImageFile_RejectsUnsafeInput covers data imagesafe
// cannot even parse as an image: refused, and dst is never written.
func TestReencodeImageFile_RejectsUnsafeInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "original.bin")
	if err := os.WriteFile(src, []byte("not an image at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "view.jpg")

	if _, _, _, err := connector.ReencodeImageFile(src, dst, imagesafe.Thumbnail); !errors.Is(err, connector.ErrUnsafeImage) {
		t.Fatalf("ReencodeImageFile() = %v, want %v", err, connector.ErrUnsafeImage)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("unsafe input still produced a destination file")
	}
}

// TestReencodeImageFile_RejectsOverProfileSizeBeforeReading covers
// the cheapest rejection of all: src's own size on disk, over
// profile's MaxInputLen, is refused from a stat alone. The file holds
// no real image bytes at all, so a pass here could only mean the
// size was checked first - a decode attempt would instead fail with
// ErrUnsafeImage, not ErrMediaTooLarge.
func TestReencodeImageFile_RejectsOverProfileSizeBeforeReading(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "huge.bin")
	huge := make([]byte, imagesafe.Thumbnail.MaxInputLen+1)
	if err := os.WriteFile(src, huge, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "view.jpg")

	_, _, _, err := connector.ReencodeImageFile(src, dst, imagesafe.Thumbnail)
	if !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("ReencodeImageFile() = %v, want %v", err, connector.ErrMediaTooLarge)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("an oversized source still produced a destination file")
	}
}

// TestReencodeImageFile_ReportsAMissingSourceFile covers src simply
// not existing: a plain error, never one of connector's own
// sentinels, which both name a problem with the bytes themselves.
func TestReencodeImageFile_ReportsAMissingSourceFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, _, _, err := connector.ReencodeImageFile(filepath.Join(dir, "missing.png"), filepath.Join(dir, "view.jpg"), imagesafe.Thumbnail)
	if err == nil {
		t.Fatal("ReencodeImageFile() on a missing source succeeded")
	}
	if errors.Is(err, connector.ErrUnsafeImage) {
		t.Error("a missing file was reported as unsafe rather than as a plain read error")
	}
}

// TestReencodeImageFile_ReportsAWriteFailure covers the destination
// write itself failing: a directory that does not exist, so an
// otherwise-valid re-encode still cannot be saved.
func TestReencodeImageFile_ReportsAWriteFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "original.png")
	if err := os.WriteFile(src, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := connector.ReencodeImageFile(src, filepath.Join(dir, "no-such-directory", "view.jpg"), imagesafe.Thumbnail)
	if err == nil {
		t.Fatal("ReencodeImageFile() = nil, want an error for a destination directory that does not exist")
	}
}

// TestSanitizeOutgoingImageFile_ChecksSizeFormatAndRewritesInPlace
// covers every outcome app.describeAttachment's own sanitizeAttachedPhoto
// depends on: a real image is rewritten in place through imagesafe; a
// file declaring more bytes than imagesafe.Outgoing accepts is
// refused before anything reads it; bytes that merely sniff as an
// image but fail imagesafe's own parser are refused too; and a
// missing file is a plain error, none of connector's own sentinels.
func TestSanitizeOutgoingImageFile_ChecksSizeFormatAndRewritesInPlace(t *testing.T) {
	t.Parallel()

	t.Run("a real image is sanitized in place", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "out.png")
		if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := connector.SanitizeOutgoingImageFile(path); err != nil {
			t.Fatalf("SanitizeOutgoingImageFile() = %v, want nil", err)
		}

		safe, err := os.ReadFile(path)
		if err != nil || len(safe) == 0 {
			t.Fatalf("file on disk = %v, %v, want a sanitized image", safe, err)
		}
	})

	t.Run("a declared size over the profile's cap is refused before reading it in full", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "huge.png")
		huge := append([]byte{0x89, 0x50, 0x4E, 0x47}, make([]byte, imagesafe.Outgoing.MaxInputLen+1)...)
		if err := os.WriteFile(path, huge, 0o600); err != nil {
			t.Fatal(err)
		}

		err := connector.SanitizeOutgoingImageFile(path)
		if !errors.Is(err, connector.ErrMediaTooLarge) {
			t.Fatalf("SanitizeOutgoingImageFile() = %v, want %v", err, connector.ErrMediaTooLarge)
		}
	})

	t.Run("bytes that are not really a valid image are refused", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "evil.png")
		if err := os.WriteFile(path, []byte("not an image at all"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := connector.SanitizeOutgoingImageFile(path)
		if !errors.Is(err, connector.ErrUnsafeImage) {
			t.Fatalf("SanitizeOutgoingImageFile() = %v, want %v", err, connector.ErrUnsafeImage)
		}
	})

	t.Run("a missing file is a plain error", func(t *testing.T) {
		t.Parallel()

		err := connector.SanitizeOutgoingImageFile(filepath.Join(t.TempDir(), "missing.png"))
		if err == nil {
			t.Fatal("SanitizeOutgoingImageFile() on a missing file succeeded")
		}
		if errors.Is(err, connector.ErrMediaTooLarge) || errors.Is(err, connector.ErrUnsafeImage) {
			t.Errorf("err = %v, want a plain stat error, not one of connector's own sentinels", err)
		}
	})
}

// TestSanitizeOutgoingImage_AcceptsAndDownscalesAPhotoOverFullImagesCap
// covers the regression FullImage's own lower cap introduced:
// SanitizeOutgoingImage must be wired to imagesafe.Outgoing, not
// FullImage, so a real camera photo over FullImage's 4096-pixel side
// is still accepted for sending, shrunk to fit rather than refused.
func TestSanitizeOutgoingImage_AcceptsAndDownscalesAPhotoOverFullImagesCap(t *testing.T) {
	t.Parallel()

	data := fastSolidJPEG(5000, 4000) // over FullImage's own MaxSide, under Outgoing's

	safe, err := connector.SanitizeOutgoingImage(data)
	if err != nil {
		t.Fatalf("SanitizeOutgoingImage() = %v, want nil: a 5000x4000 photo must be accepted under Outgoing", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(safe))
	if err != nil {
		t.Fatalf("decode result config: %v", err)
	}
	if cfg.Width > 4096 || cfg.Height > 4096 {
		t.Errorf("result = %dx%d, want at most 4096 on each side after downscaling", cfg.Width, cfg.Height)
	}
}

// TestSanitizeOutgoingImageFile_AcceptsAndDownscalesAPhotoOverFullImagesCap
// mirrors the above for the on-disk path app.describeAttachment's own
// sanitizeAttachedPhoto actually calls.
func TestSanitizeOutgoingImageFile_AcceptsAndDownscalesAPhotoOverFullImagesCap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, fastSolidJPEG(5000, 4000), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := connector.SanitizeOutgoingImageFile(path); err != nil {
		t.Fatalf("SanitizeOutgoingImageFile() = %v, want nil: a 5000x4000 photo must be accepted under Outgoing", err)
	}

	safe, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(safe))
	if err != nil {
		t.Fatalf("decode result config: %v", err)
	}
	if cfg.Width > 4096 || cfg.Height > 4096 {
		t.Errorf("result = %dx%d, want at most 4096 on each side after downscaling", cfg.Width, cfg.Height)
	}
}

// TestSaveMedia_WritesUnchanged covers a video, audio, document, or
// now a photo or sticker download too (see SaveMedia's own doc
// comment): whatever bytes a connector downloaded land on disk
// exactly as they were, with no check of their own at write time.
func TestSaveMedia_WritesUnchanged(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "out.bin")
	if err := connector.SaveMedia(path, []byte("whatever bytes, unchanged")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "whatever bytes, unchanged" {
		t.Errorf("read %q, %v, want the bytes written unchanged", got, err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, %v, want 0600", info, err)
	}
}
