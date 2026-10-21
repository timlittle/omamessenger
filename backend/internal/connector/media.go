package connector

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// ErrUnsafeImage reports that a photo, sticker or GIF a connector
// downloaded failed its safety check (see imagesafe) and so was never
// written to disk at all. The app layer reports this the same way as
// any other failed fetch: the UI falls back to a thumbnail or a
// placeholder, never a crash.
var ErrUnsafeImage = errors.New("connector: image failed safety check")

// SaveImageMedia strips compressed or oversized metadata from data,
// which must be a photo, sticker or GIF a connector is about to let
// the UI load into a QML Image or AnimatedImage, and writes the
// result to path, readable only by the user. A sender-controlled
// image is the one kind of downloaded media Qt itself decodes inline,
// which is why every connector's FetchMedia routes exactly these
// kinds through here rather than SaveMedia (see docs/decisions.md).
func SaveImageMedia(path string, data []byte, profile imagesafe.Profile) error {
	safe, ok := imagesafe.Sanitize(data, profile)
	if !ok {
		return fmt.Errorf("connector: save image media: %w", ErrUnsafeImage)
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil {
		return fmt.Errorf("connector: save image media: %w", err)
	}

	return nil
}

// SanitizeOutgoingImage strips compressed or oversized metadata from
// data, an image the user is attaching to an outgoing message - a
// paste from the clipboard, say - and returns the sanitized bytes
// without writing anything to disk: the app layer copies the result
// into the outgoing media area itself. A pasted image comes from
// whichever other program last put something on the clipboard, so it
// is checked against the same full-size profile a downloaded photo
// is, never trusted just because the user chose to paste it.
func SanitizeOutgoingImage(data []byte) ([]byte, error) {
	safe, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
	if !ok {
		return nil, fmt.Errorf("connector: sanitize outgoing image: %w", ErrUnsafeImage)
	}

	return safe, nil
}

// SaveMedia writes data to path unchanged, readable only by the
// user, for a video, audio, voice note, file or document: formats
// Qt's own image decoders never touch, so none of SaveImageMedia's
// risk applies.
func SaveMedia(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("connector: save media: %w", err)
	}

	return nil
}

// gifMagic is the byte sequence a genuine GIF file begins with
// ("GIF87a" or "GIF89a"). SanitizeIfGIF checks a file's own bytes
// against it directly; a sender's filename, mime type or "animated"
// attribute is never enough on its own to pick an image decoder over a
// video one (see docs/decisions.md).
var gifMagic = []byte("GIF8")

// SanitizeIfGIF re-examines path, a video or other document FetchMedia
// has already written to disk unchanged because the sender's own
// attributes said it was not an image. When path's actual first bytes
// are a genuine GIF header, it rewrites the file in place through
// imagesafe's FullImage profile and reports true, so the UI can trust
// decoding it as an animated image; a file whose bytes fail that
// sanitizer, whether malformed or over its caps, is removed and
// reported as an error rather than left half-trusted on disk. Anything
// whose header does not match, or whose size alone is already larger
// than FullImage's own input cap, is left exactly as it was and
// reported false: a real video, which SanitizeIfGIF never reads in
// full, let alone sanitizes.
func SanitizeIfGIF(path string) (isGIF bool, err error) {
	head, size, err := peekFile(path, len(gifMagic))
	if err != nil {
		return false, fmt.Errorf("connector: sanitize if gif: %w", err)
	}
	if !bytes.Equal(head, gifMagic) {
		return false, nil
	}
	if size > imagesafe.FullImage.MaxInputLen {
		return false, fmt.Errorf("connector: sanitize if gif: %w", errors.Join(ErrMediaTooLarge, removeIfPresent(path)))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("connector: sanitize if gif: %w", err)
	}

	safe, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
	if !ok {
		return false, fmt.Errorf("connector: sanitize if gif: %w", errors.Join(ErrUnsafeImage, removeIfPresent(path)))
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil { //nolint:gosec // deliberate: path is the media cache path FetchMedia already wrote to and this function only re-reads; it is never built from untrusted input
		return false, fmt.Errorf("connector: sanitize if gif: %w", err)
	}

	return true, nil
}

// peekFile reads up to n leading bytes of path and reports the file's
// total size, for SanitizeIfGIF's two checks: what the file actually
// starts with, and whether it is small enough to read in full at all.
func peekFile(path string, n int) (head []byte, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close() // reading only; nothing to flush

	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}

	head = make([]byte, n)
	read, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, 0, err
	}

	return head[:read], info.Size(), nil
}

// removeIfPresent deletes path, treating one already gone as removed:
// the best-effort cleanup for a file SanitizeIfGIF refuses after it was
// already written to disk by the ordinary, unsanitized download path.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
