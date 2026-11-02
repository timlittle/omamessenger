package connector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// ErrUnsafeImage reports that a photo, sticker or GIF failed
// imagesafe's own check: a decode that found the bytes malformed,
// panicked, or produced a size over its profile's caps. The app layer
// reports this the same way as any other failed fetch: the UI falls
// back to a thumbnail or a placeholder, never a crash.
var ErrUnsafeImage = errors.New("connector: image failed safety check")

// SanitizeOutgoingImage re-encodes data, an image the user is
// attaching to an outgoing message - a paste from the clipboard, say -
// through imagesafe.Reencode under the full-size profile, and returns
// the result without writing anything to disk: the app layer copies
// it into the outgoing media area itself. A pasted image comes from
// whichever other program last put something on the clipboard, so it
// is decoded and re-encoded the same way a downloaded photo is, never
// trusted just because the user chose to paste it (see
// docs/decisions.md): this is the one point in an outgoing photo's
// whole life this package ever re-encodes it, so both the composer's
// own preview and the bubble shown once it sends can keep reusing the
// same already-safe bytes afterwards.
func SanitizeOutgoingImage(data []byte) ([]byte, error) {
	safe, _, _, _, ok := imagesafe.Reencode(data, imagesafe.FullImage)
	if !ok {
		return nil, fmt.Errorf("connector: sanitize outgoing image: %w", ErrUnsafeImage)
	}

	return safe, nil
}

// SanitizeOutgoingImageFile overwrites path in place with
// SanitizeOutgoingImage's own re-encoded rewrite of its own content,
// under the same full-size profile: for a photo attachment the app
// layer has already copied into the outgoing media area from a file
// the user picked on disk, decoded and re-encoded the same way before
// anything reads it further, never trusted further just because the
// user chose to attach it (see docs/decisions.md). path's declared
// size is checked against that profile's own input cap before the
// file is ever read into memory, the same discipline a downloaded
// photo's declared size already gets, so an attachment many times
// larger than any real photo cannot force an unbounded read just to
// find out it will be refused.
func SanitizeOutgoingImageFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}
	if info.Size() > imagesafe.FullImage.MaxInputLen {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", ErrMediaTooLarge)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	safe, err := SanitizeOutgoingImage(data)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil { //nolint:gosec // deliberate: path is the outgoing media area path the app layer already wrote this same file to; it is never built from untrusted input
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	return nil
}

// SaveMedia writes data to path unchanged, readable only by the
// user. Every connector's FetchMedia uses this for a video, audio,
// voice note or file - formats Qt's own image decoders never touch -
// and, now that the UI loads a photo, sticker or GIF's own display
// copy instead of this file (see ReencodeImageFile and
// docs/decisions.md), for one of those downloads too: the one check
// that still matters for them is Reencode's own, run once when their
// display copy is first built, never again on every write of the
// original.
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
// imagesafe's AnimatedGIF profile and reports true, so the UI can
// trust decoding it as an animated image; a file whose bytes fail
// that sanitizer, whether malformed or over its caps, is removed and
// reported as an error rather than left half-trusted on disk. Anything
// whose header does not match, or whose size alone is already larger
// than AnimatedGIF's own input cap, is left exactly as it was and
// reported false: a real video, which SanitizeIfGIF never reads in
// full, let alone sanitizes. width and height are the file's own
// canvas size, as imagesafe.Sanitize's own parse of it just confirmed
// (0, 0 whenever isGIF is false), for app.Commands to hand on to the
// UI in place of the sender's claimed video width/height: a video
// marked to play back like a silent "GIF" carries only that claim,
// which need not match what the file playing through AnimatedImage
// actually decodes to (see docs/decisions.md).
func SanitizeIfGIF(path string) (isGIF bool, width, height int, err error) {
	head, size, err := peekFile(path, len(gifMagic))
	if err != nil {
		return false, 0, 0, fmt.Errorf("connector: sanitize if gif: %w", err)
	}
	if !bytes.Equal(head, gifMagic) {
		return false, 0, 0, nil
	}
	if size > imagesafe.AnimatedGIF.MaxInputLen {
		return false, 0, 0, fmt.Errorf("connector: sanitize if gif: %w", errors.Join(ErrMediaTooLarge, removeIfPresent(path)))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, 0, 0, fmt.Errorf("connector: sanitize if gif: %w", err)
	}

	safe, width, height, ok := imagesafe.Sanitize(data, imagesafe.AnimatedGIF)
	if !ok {
		return false, 0, 0, fmt.Errorf("connector: sanitize if gif: %w", errors.Join(ErrUnsafeImage, removeIfPresent(path)))
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil { //nolint:gosec // deliberate: path is the media cache path FetchMedia already wrote to and this function only re-reads; it is never built from untrusted input
		return false, 0, 0, fmt.Errorf("connector: sanitize if gif: %w", err)
	}

	return true, width, height, nil
}

// ReencodeImageFile reads srcPath - a photo, sticker or GIF this
// helper already has on disk, downloaded or attached, read here but
// never written to - fully through imagesafe.Reencode under profile,
// and writes the result to dstPath, readable only by the user:
// Qt's own C image decoders inside omarchy-shell only ever see bytes
// this call's own encoder wrote, never srcPath's own bytes directly
// (see docs/decisions.md). app.Commands calls this exactly once to
// fill a display copy's own cache slot, never on every time that
// slot is served afterwards, since Reencode is a lossy, one-way
// rewrite that must not run again on its own earlier output. format,
// width and height are Reencode's own result, for the caller to name
// and size that cache slot from; ok is false for the same reasons
// imagesafe.Reencode's own ok is, or srcPath cannot be read, and
// dstPath is never written in that case.
func ReencodeImageFile(srcPath, dstPath string, profile imagesafe.Profile) (format imagesafe.OutputFormat, width, height int, err error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}

	out, format, width, height, ok := imagesafe.Reencode(data, profile)
	if !ok {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", ErrUnsafeImage)
	}

	if err := os.WriteFile(dstPath, out, 0o600); err != nil { //nolint:gosec // deliberate: dstPath is a display copy's cache path app.Commands already builds through cache.FetchNamed, never built from untrusted input
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}

	return format, width, height, nil
}

// VideoPlayable reports whether path, an ordinary video FetchMedia has
// already written to disk unchanged (never one SanitizeIfGIF already
// confirmed is actually a GIF image), is safe to hand to the UI's own
// in-window video player at all: its container and stream headers,
// read in pure Go without ever shelling out or reading its sample
// data (see videosafe), declare an allowed codec, dimensions and
// duration within this package's limits, and, if it carries audio, an
// allowed audio codec too. False, with no error, covers every way a
// video fails that check - the sender's own bytes turned out to be
// something this UI must never be trusted to decode in place - while
// an error here means path itself could not be read at all. ctx lets
// the caller give up on the check - the UI closing the conversation
// before a fetch finishes, say - without waiting out whatever is left
// of it (see videosafe.ValidateContext).
func VideoPlayable(ctx context.Context, path string) (bool, error) {
	playable, err := videosafe.ValidateContext(ctx, path)
	if err != nil {
		return false, fmt.Errorf("connector: video playable: %w", err)
	}

	return playable, nil
}

// AudioPlayable reports whether path, a voice note or audio file
// FetchMedia has already written to disk unchanged, is safe to hand
// to the UI's own in-window audio player at all: its own bytes, read
// in pure Go without ever shelling out (see audiosafe), describe a
// single-stream Ogg/Opus or MP4/M4A (AAC or Opus, no video track)
// file within audiosafe's own duration limit. MP3 (and other MPEG
// audio) is never playable in-window; see audiosafe's own package
// doc comment and docs/decisions.md. False, with no error, covers
// every way an audio file fails that check - the sender's own bytes
// turned out to be something this UI must never be trusted to decode
// in place - while an error here means path itself could not be read
// at all.
func AudioPlayable(path string) (bool, error) {
	playable, err := audiosafe.Validate(path)
	if err != nil {
		return false, fmt.Errorf("connector: audio playable: %w", err)
	}

	return playable, nil
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
