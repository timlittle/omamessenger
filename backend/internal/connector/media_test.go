package connector_test

// media_test.go checks SaveImageMedia and SaveMedia directly, and
// guards the invariant every connector's FetchMedia depends on: a
// photo, sticker or GIF's downloaded bytes always reach disk through
// SaveImageMedia, which runs them through imagesafe first, never
// through a raw file write that could put a sender's unchecked bytes
// straight in front of a QML Image component.

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestSaveImageMedia_SanitizesOrRejects covers both outcomes: real
// image bytes are written, through imagesafe, readable only by the
// user; anything imagesafe rejects is never written at all, and the
// caller sees ErrUnsafeImage.
func TestSaveImageMedia_SanitizesOrRejects(t *testing.T) {
	t.Parallel()

	t.Run("a real image is written", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "out.png")
		if err := connector.SaveImageMedia(path, onePixelPNG(), imagesafe.Thumbnail); err != nil {
			t.Fatalf("SaveImageMedia() = %v, want a sanitized file written", err)
		}

		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, %v, want 0600", info, err)
		}
	})

	t.Run("unsafe bytes are rejected and never written", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "out.png")
		err := connector.SaveImageMedia(path, []byte("not an image at all"), imagesafe.Thumbnail)
		if !errors.Is(err, connector.ErrUnsafeImage) {
			t.Fatalf("SaveImageMedia() = %v, want %v", err, connector.ErrUnsafeImage)
		}
		if _, err := os.Stat(path); err == nil {
			t.Fatal("unsafe bytes were written to disk anyway")
		}
	})
}

// TestSanitizeIfGIF_OnlyTrustsTheFileSOwnBytes reproduces the bug a
// video Telegram document carrying both a video attribute and an
// animated attribute could exploit: a sender who marks a document a
// video (so FetchMedia streams it to disk unchanged, never through
// SaveImageMedia) but gives it a ".gif" name and the animated flag can
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

		isGIF, err := connector.SanitizeIfGIF(path)
		if err != nil || !isGIF {
			t.Fatalf("SanitizeIfGIF() = %t, %v, want true, nil", isGIF, err)
		}

		safe, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := imagesafe.Sanitize(safe, imagesafe.FullImage); !ok {
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

		isGIF, err := connector.SanitizeIfGIF(path)
		if err != nil || isGIF {
			t.Fatalf("SanitizeIfGIF() = %t, %v, want false, nil", isGIF, err)
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

		isGIF, err := connector.SanitizeIfGIF(path)
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

		_, err := connector.SanitizeIfGIF(filepath.Join(t.TempDir(), "missing.gif"))
		if err == nil {
			t.Fatal("SanitizeIfGIF() on a missing file succeeded")
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

		playable, err := connector.VideoPlayable(path)
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

		playable, err := connector.VideoPlayable(path)
		if err != nil || playable {
			t.Fatalf("VideoPlayable() = %t, %v, want false, nil", playable, err)
		}
	})

	t.Run("a missing file is reported as an error", func(t *testing.T) {
		t.Parallel()

		_, err := connector.VideoPlayable(filepath.Join(t.TempDir(), "missing.mp4"))
		if err == nil {
			t.Fatal("VideoPlayable() on a missing file succeeded")
		}
	})
}

// TestSaveMedia_WritesUnchanged covers a video, audio or document: none
// of SaveImageMedia's checks apply, so whatever bytes a connector
// downloaded land on disk exactly as they were.
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

// rawFileWrite matches a direct call to os.WriteFile or os.OpenFile:
// the two ways a package could put bytes on disk without going
// through SaveImageMedia or SaveMedia.
var rawFileWrite = regexp.MustCompile(`\bos\.(WriteFile|OpenFile)\(`)

// sharedSaverCall matches a call to one of this package's own shared
// savers, so the guard test below can confirm a connector actually
// uses one rather than merely not using a raw file write.
var sharedSaverCall = regexp.MustCompile(`connector\.Save(Image)?Media\(`)

// funcHeader matches a top-level function declaration's own line, to
// tell which function a later match in the same file falls under.
var funcHeader = regexp.MustCompile(`(?m)^func (?:\([^)]*\)\s*)?(\w+)\(`)

// rawWriteAllowlist names the one function per connector directory
// allowed a raw file write: a video, audio or document never reaches
// a QML Image, so streaming it straight to disk - rather than
// buffering it in memory only to call SaveMedia - costs nothing in
// safety. Every other function, especially the one that decides
// whether a download is an image at all, must go through this
// package's shared savers instead.
var rawWriteAllowlist = map[string]string{
	"telegram": "download",
}

// TestConnectors_WriteFetchedMediaOnlyThroughSharedSavers statically
// checks fetch.go in the whatsapp and telegram connector packages -
// the one file in each where FetchMedia decides how to write a
// message's downloaded media to disk: every raw os.WriteFile or
// os.OpenFile call falls under rawWriteAllowlist's one named
// exception, if the directory has one, and each file calls
// SaveImageMedia or SaveMedia at least once. A future change that
// reached for a raw file write anywhere else - the exact mistake a
// marketplace review found in this package's sticker path - fails
// this test rather than reaching production unnoticed.
func TestConnectors_WriteFetchedMediaOnlyThroughSharedSavers(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{"whatsapp", "telegram"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(dir, "fetch.go")
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			for _, fn := range functionsContaining(src, rawFileWrite) {
				if fn != rawWriteAllowlist[dir] {
					t.Errorf("%s: func %s calls os.WriteFile or os.OpenFile directly; use connector.SaveImageMedia or connector.SaveMedia instead", path, fn)
				}
			}
			if !sharedSaverCall.Match(src) {
				t.Errorf("%s never calls connector.SaveImageMedia or connector.SaveMedia at all", path)
			}
		})
	}
}

// functionsContaining returns the name of every top-level function in
// src whose body matches pattern at least once.
func functionsContaining(src []byte, pattern *regexp.Regexp) []string {
	headers := funcHeader.FindAllSubmatchIndex(src, -1)
	var names []string

	for i, h := range headers {
		end := len(src)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		if pattern.Match(src[h[0]:end]) {
			names = append(names, string(src[h[2]:h[3]]))
		}
	}

	return names
}

// TestFunctionsContaining_FindsAndSkipsFunctions proves the helper the
// guard test above depends on actually attributes a match to the
// right function, and reports none for a function that has no match
// at all, rather than silently checking nothing.
func TestFunctionsContaining_FindsAndSkipsFunctions(t *testing.T) {
	t.Parallel()

	src := []byte(`package x

func clean() {
	fmt.Println("nothing to see here")
}

func dirty(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
`)

	got := functionsContaining(src, rawFileWrite)
	if len(got) != 1 || got[0] != "dirty" {
		t.Errorf("functionsContaining() = %v, want [dirty]", got)
	}
}

// TestRawFileWrite_CatchesADeliberateViolation proves the regexp
// above actually matches what it is meant to catch, rather than
// silently matching nothing.
func TestRawFileWrite_CatchesADeliberateViolation(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		`os.WriteFile(path, data, 0o600)`:           true,
		`os.OpenFile(path, os.O_CREATE, 0o600)`:     true,
		`connector.SaveImageMedia(path, data, p)`:   false,
		`// os.WriteFile(x, y, z) is disabled here`: true, // the check is textual, on purpose: see its own doc comment
	}

	for src, want := range cases {
		if got := rawFileWrite.MatchString(src); got != want {
			t.Errorf("rawFileWrite.MatchString(%q) = %v, want %v", src, got, want)
		}
	}
}
