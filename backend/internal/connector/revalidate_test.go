package connector_test

// revalidate_test.go covers RevalidateDownloadedImage and
// RevalidateOutgoingImage: the one-time sweep a helper upgrading past
// an older version of imagesafe's checks runs over every file already
// on disk, closing the gap a plain cache hit would otherwise leave
// (see docs/decisions.md). Every other media kind these two areas
// hold - a video, voice note or file - already gets its own
// kind-specific check on every single fetch regardless of age (see
// SanitizeIfGIF, VideoPlayable, AudioPlayable and app.MediaRisk), so
// these tests cover only what is specific to an image: kept,
// rewritten or removed, and never touching anything that is not an
// image at all.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// pngWithTextChunk returns a tiny, real PNG with an extra tEXt chunk
// inserted right after its IHDR - exactly the kind of ancillary chunk
// imagesafe.Sanitize strips - carrying marker in its payload, so a
// test can confirm the sanitized rewrite actually removed it.
func pngWithTextChunk(marker string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	src := buf.Bytes()

	ihdrEnd := 8 + 8 + 13 + 4 // signature + length/type + IHDR body + CRC
	var out bytes.Buffer
	out.Write(src[:ihdrEnd])
	writePNGChunk(&out, "tEXt", []byte("Comment\x00"+marker))
	out.Write(src[ihdrEnd:])

	return out.Bytes()
}

// pngWithOversizedIHDR returns a tiny, real PNG whose IHDR is
// rewritten to declare a far larger width and height than it actually
// encodes, the same bomb imagesafe.Sanitize's own dimension check
// exists to reject.
func pngWithOversizedIHDR() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	out := append([]byte{}, buf.Bytes()...)

	binary.BigEndian.PutUint32(out[16:20], 50_000)
	binary.BigEndian.PutUint32(out[20:24], 50_000)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(out[12:29]))
	copy(out[29:33], crc[:])

	return out
}

// writePNGChunk appends one well-formed PNG chunk - length, type,
// payload and a correct CRC - to out.
func writePNGChunk(out *bytes.Buffer, kind string, payload []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	out.Write(length[:])
	out.WriteString(kind)
	out.Write(payload)

	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(append([]byte(kind), payload...)))
	out.Write(crc[:])
}

// TestRevalidateDownloadedImage_KeepsAnAlreadySafeFile covers a file
// imagesafe.Sanitize already accepts unchanged: left exactly as it
// was, not rewritten for no reason.
func TestRevalidateDownloadedImage_KeepsAnAlreadySafeFile(t *testing.T) {
	t.Parallel()

	for name, data := range map[string][]byte{"photo": onePixelPNG(), "gif": onePixelGIF()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "m1.jpg")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}

			keep, err := connector.RevalidateDownloadedImage(path)
			if err != nil || !keep {
				t.Fatalf("RevalidateDownloadedImage() = %t, %v, want true, nil", keep, err)
			}

			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, data) {
				t.Error("an already-safe file was rewritten, want it left exactly as it was")
			}
		})
	}
}

// TestRevalidateDownloadedImage_RewritesAFileSanitizeStrips covers a
// legacy file that still passes today's checks but carries something
// Sanitize removes: kept, but rewritten to the stripped bytes, the
// same outcome a fresh download of the same image would produce today.
func TestRevalidateDownloadedImage_RewritesAFileSanitizeStrips(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "m1.jpg")
	original := pngWithTextChunk("legacy metadata nobody asked for")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	keep, err := connector.RevalidateDownloadedImage(path)
	if err != nil || !keep {
		t.Fatalf("RevalidateDownloadedImage() = %t, %v, want true, nil", keep, err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, original) {
		t.Error("file was not rewritten, want the stripped chunk gone")
	}
	if bytes.Contains(got, []byte("legacy metadata")) {
		t.Error("stripped chunk's own text is still on disk")
	}
	if _, _, _, ok := imagesafe.Sanitize(got, imagesafe.FullImage); !ok {
		t.Error("rewritten file no longer sanitizes cleanly")
	}
}

// TestRevalidateDownloadedImage_RemovesWhatStillFailsSanitize covers
// the actual vulnerability this sweep closes: a file that was never
// sanitized by the version of this helper that wrote it, and still
// fails today's check, reported false so the caller removes it
// rather than ever serving it to the UI.
func TestRevalidateDownloadedImage_RemovesWhatStillFailsSanitize(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "m1.jpg")
	if err := os.WriteFile(path, pngWithOversizedIHDR(), 0o600); err != nil {
		t.Fatal(err)
	}

	keep, err := connector.RevalidateDownloadedImage(path)
	if err != nil || keep {
		t.Fatalf("RevalidateDownloadedImage() = %t, %v, want false, nil", keep, err)
	}
}

// TestRevalidateDownloadedImage_LeavesNonImagesAlone covers a file
// whose own bytes are not one of the formats imagesafe recognizes at
// all - a video, voice note or plain document the downloaded media
// cache also holds: left completely untouched, since it was never
// going to reach Qt's image decoder in the first place, and already
// gets its own kind-specific check on every fetch regardless (see
// this file's own header comment).
func TestRevalidateDownloadedImage_LeavesNonImagesAlone(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "m1.mp4")
	original := []byte("not an image at all, whatever its cache name claims")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	keep, err := connector.RevalidateDownloadedImage(path)
	if err != nil || !keep {
		t.Fatalf("RevalidateDownloadedImage() = %t, %v, want true, nil", keep, err)
	}

	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Error("a non-image file was changed, want it left exactly as it was")
	}
}

// TestRevalidateOutgoingImage_SanitizesThroughTheSameFullSizeProfile
// covers the outgoing attachment area's own version of the sweep: it
// runs the exact same full-size sanitizer every outgoing attachment
// is already sanitized through today (SanitizeOutgoingImageFile), so
// a legacy attachment copy, from before that existed, is held to the
// same bar a fresh attachment gets.
func TestRevalidateOutgoingImage_SanitizesThroughTheSameFullSizeProfile(t *testing.T) {
	t.Parallel()

	t.Run("a safe file is kept", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "m1-photo.jpg")
		if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
			t.Fatal(err)
		}

		keep, err := connector.RevalidateOutgoingImage(path)
		if err != nil || !keep {
			t.Fatalf("RevalidateOutgoingImage() = %t, %v, want true, nil", keep, err)
		}
	})

	t.Run("a file that still fails sanitizing is removed by the caller", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "m1-photo.jpg")
		if err := os.WriteFile(path, pngWithOversizedIHDR(), 0o600); err != nil {
			t.Fatal(err)
		}

		keep, err := connector.RevalidateOutgoingImage(path)
		if err != nil || keep {
			t.Fatalf("RevalidateOutgoingImage() = %t, %v, want false, nil", keep, err)
		}
	})

	t.Run("a non-image attachment is left alone", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "m1-notes.txt")
		original := []byte("a plain text file someone attached")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}

		keep, err := connector.RevalidateOutgoingImage(path)
		if err != nil || !keep {
			t.Fatalf("RevalidateOutgoingImage() = %t, %v, want true, nil", keep, err)
		}

		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, original) {
			t.Error("a non-image attachment was changed, want it left exactly as it was")
		}
	})
}

// TestRevalidateDownloadedImage_ReportsAnUnreadableFile covers a path
// that cannot even be opened: reported as an error, never silently
// kept.
func TestRevalidateDownloadedImage_ReportsAnUnreadableFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.jpg")

	if _, err := connector.RevalidateDownloadedImage(path); err == nil {
		t.Fatal("RevalidateDownloadedImage() = nil error, want one for a file that cannot be read")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}

// TestRevalidateDownloadedImage_RemovesAFileItCannotRead covers the
// other half of the same guard: a file that exists but cannot even be
// opened - here, every permission bit cleared - is removed rather
// than left trusted on disk just because this sweep failed to look at
// it (see docs/decisions.md).
func TestRevalidateDownloadedImage_RemovesAFileItCannotRead(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	path := filepath.Join(t.TempDir(), "m1.jpg")
	if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}

	if _, err := connector.RevalidateDownloadedImage(path); err == nil {
		t.Fatal("RevalidateDownloadedImage() = nil error, want one for a file that cannot be read")
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unreadable file stat = %v, want it removed", err)
	}
}

// TestRevalidateOutgoingImage_RemovesAFileItCannotRead mirrors
// TestRevalidateDownloadedImage_RemovesAFileItCannotRead for the
// outgoing attachment area's own half of the sweep.
func TestRevalidateOutgoingImage_RemovesAFileItCannotRead(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	path := filepath.Join(t.TempDir(), "m1-photo.jpg")
	if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}

	if _, err := connector.RevalidateOutgoingImage(path); err == nil {
		t.Fatal("RevalidateOutgoingImage() = nil error, want one for a file that cannot be read")
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unreadable file stat = %v, want it removed", err)
	}
}

// skipAsRoot skips a test that relies on file permissions refusing
// access: root bypasses them, so as root (as CI's container runs) the
// failure the test expects can never happen.
func skipAsRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict root")
	}
}
