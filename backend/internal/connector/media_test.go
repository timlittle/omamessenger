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

// TestSanitizeIfGIF_OnlyTrustsTheFileSOwnBytes reproduces the bug a
// video Telegram document carrying both a video attribute and an
// animated attribute could exploit: a sender who marks a document a
// video (so FetchMedia streams it to disk unchanged through
// SaveMedia, never imagesafe-checked at write time) but gives it a
// ".gif" name and the animated flag can
// make the UI pick AnimatedImage purely from that metadata. The fix is
// this check, run after such a file is already on disk: it never
// reads a filename, mime type or attribute, only the file's own first
// bytes, so a video that merely claims to be a GIF is left exactly as
// it was, while bytes that really are GIF-format are sanitized in
// place and reported as such.
func TestSanitizeIfGIF_OnlyTrustsTheFileSOwnBytes(t *testing.T) {
	t.Parallel()

	t.Run("a real GIF is sanitized in place and reported true", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "evil.gif")
		if err := os.WriteFile(path, onePixelGIF(), 0o600); err != nil {
			t.Fatal(err)
		}

		isGIF, width, height, err := connector.SanitizeIfGIF(path)
		if err != nil || !isGIF {
			t.Fatalf("SanitizeIfGIF() = %t, %v, want true, nil", isGIF, err)
		}
		if width != 1 || height != 1 {
			t.Errorf("SanitizeIfGIF() width/height = %d/%d, want the real 1x1 canvas", width, height)
		}

		safe, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, ok := imagesafe.Sanitize(safe, imagesafe.FullImage); !ok {
			t.Error("the file on disk is not sanitized, valid GIF bytes")
		}
	})

	t.Run("a video merely named and flagged like a GIF is left untouched and reported false", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "evil.gif")
		original := []byte("not a gif at all, whatever its name or attributes claim")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}

		isGIF, width, height, err := connector.SanitizeIfGIF(path)
		if err != nil || isGIF {
			t.Fatalf("SanitizeIfGIF() = %t, %v, want false, nil", isGIF, err)
		}
		if width != 0 || height != 0 {
			t.Errorf("SanitizeIfGIF() width/height = %d/%d, want 0/0 when it is not a GIF", width, height)
		}

		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(original) {
			t.Errorf("file changed to %q, %v, want it left exactly as it was", got, err)
		}
	})

	t.Run("GIF bytes over imagesafe's own caps are refused, not silently left as a video", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "bomb.gif")
		huge := append([]byte("GIF89a"), make([]byte, imagesafe.FullImage.MaxInputLen)...)
		if err := os.WriteFile(path, huge, 0o600); err != nil {
			t.Fatal(err)
		}

		isGIF, _, _, err := connector.SanitizeIfGIF(path)
		if err == nil || isGIF {
			t.Fatalf("SanitizeIfGIF() = %t, %v, want an error refusing it", isGIF, err)
		}
		if !errors.Is(err, connector.ErrMediaTooLarge) {
			t.Errorf("err = %v, want %v", err, connector.ErrMediaTooLarge)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("an oversized claimed GIF was left on disk")
		}
	})

	t.Run("a missing file is reported as an error", func(t *testing.T) {
		t.Parallel()

		_, _, _, err := connector.SanitizeIfGIF(filepath.Join(t.TempDir(), "missing.gif"))
		if err == nil {
			t.Fatal("SanitizeIfGIF() on a missing file succeeded")
		}
	})
}

// TestSanitizeOutgoingImageFile_ChecksSizeFormatAndRewritesInPlace
// covers every outcome app.describeAttachment's own sanitizeAttachedPhoto
// depends on: a real image is rewritten in place through imagesafe; a
// file declaring more bytes than imagesafe.FullImage accepts is
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
		huge := append([]byte{0x89, 0x50, 0x4E, 0x47}, make([]byte, imagesafe.FullImage.MaxInputLen+1)...)
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

// TestVideoPlayable_ChecksTheFilesOwnHeaders covers the two outcomes
// FetchMedia's own caller depends on: a video whose container and
// stream headers videosafe actually accepts is reported playable, and
// one that fails that check - garbage bytes here, every other way a
// header can fail is videosafe's own package to test - is reported
// not playable, with no error, since this is attacker-controlled
// data, not a program fault.
func TestVideoPlayable_ChecksTheFilesOwnHeaders(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed video is reported playable", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "video.mp4")
		if err := os.WriteFile(path, onePlayableMP4(), 0o600); err != nil {
			t.Fatal(err)
		}

		playable, err := connector.VideoPlayable(t.Context(), path)
		if err != nil || !playable {
			t.Fatalf("VideoPlayable() = %t, %v, want true, nil", playable, err)
		}
	})

	t.Run("a file with no recognizable video headers is reported not playable", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "video.mp4")
		if err := os.WriteFile(path, []byte("not a video at all, whatever its name claims"), 0o600); err != nil {
			t.Fatal(err)
		}

		playable, err := connector.VideoPlayable(t.Context(), path)
		if err != nil || playable {
			t.Fatalf("VideoPlayable() = %t, %v, want false, nil", playable, err)
		}
	})

	t.Run("a missing file is reported as an error", func(t *testing.T) {
		t.Parallel()

		_, err := connector.VideoPlayable(t.Context(), filepath.Join(t.TempDir(), "missing.mp4"))
		if err == nil {
			t.Fatal("VideoPlayable() on a missing file succeeded")
		}
	})
}

// TestAudioPlayable_ChecksTheFilesOwnHeaders covers the two outcomes
// FetchMedia's own caller depends on for a voice note, mirroring
// TestVideoPlayable_ChecksTheFilesOwnHeaders: a file whose bytes
// audiosafe actually accepts is reported playable, and one that
// fails that check is reported not playable, with no error, since
// this is attacker-controlled data, not a program fault.
func TestAudioPlayable_ChecksTheFilesOwnHeaders(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed voice note is reported playable", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "voice.ogg")
		if err := os.WriteFile(path, onePlayableOggOpus(), 0o600); err != nil {
			t.Fatal(err)
		}

		playable, err := connector.AudioPlayable(path)
		if err != nil || !playable {
			t.Fatalf("AudioPlayable() = %t, %v, want true, nil", playable, err)
		}
	})

	t.Run("a file with no recognizable audio headers is reported not playable", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "voice.ogg")
		if err := os.WriteFile(path, []byte("not audio at all, whatever its name claims"), 0o600); err != nil {
			t.Fatal(err)
		}

		playable, err := connector.AudioPlayable(path)
		if err != nil || playable {
			t.Fatalf("AudioPlayable() = %t, %v, want false, nil", playable, err)
		}
	})

	t.Run("a missing file is reported as an error", func(t *testing.T) {
		t.Parallel()

		_, err := connector.AudioPlayable(filepath.Join(t.TempDir(), "missing.ogg"))
		if err == nil {
			t.Fatal("AudioPlayable() on a missing file succeeded")
		}
	})
}

// TestSaveMedia_WritesUnchanged covers a video, audio, document, or
// now a photo, sticker or GIF download too (see SaveMedia's own doc
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
