package connector

import (
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// ErrUnsafeImage reports that a photo or sticker failed imagesafe's
// own check: a decode that found the bytes malformed, panicked, or
// produced a size over its profile's caps. The app layer reports this
// the same way as any other failed fetch: the UI falls back to a
// thumbnail or a placeholder, never a crash.
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
// voice note or file - formats Qt's own image decoders never touch,
// and a video's own in-window play copy is built from this same
// unchanged download, never run through imagesafe at all - and, now
// that the UI loads a photo or sticker's own display copy instead of
// this file (see ReencodeImageFile and docs/decisions.md), for one of
// those downloads too: the one check that still matters for them is
// Reencode's own, run once when their display copy is first built,
// never again on every write of the original.
func SaveMedia(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("connector: save media: %w", err)
	}

	return nil
}

// ReencodeImageFile reads srcPath - a photo or sticker this helper
// already has on disk, downloaded or attached, or a still frame the
// sandboxed transcoder's StillFrame profile just extracted from a
// format Go cannot decode at all, read here but never written to -
// fully through imagesafe.Reencode under profile,
// and writes the result to dstPath, readable only by the user:
// Qt's own C image decoders inside omarchy-shell only ever see bytes
// this call's own encoder wrote, never srcPath's own bytes directly
// (see docs/decisions.md). app.Commands calls this exactly once to
// fill a display copy's own cache slot, never on every time that
// slot is served afterwards, since Reencode is a lossy, one-way
// rewrite that must not run again on its own earlier output. format,
// width and height are Reencode's own result, for the caller to name
// and size that cache slot from; ok is false for the same reasons
// imagesafe.Reencode's own ok is, srcPath's own size on disk is over
// profile's MaxInputLen (checked before anything reads it, the same
// discipline SanitizeOutgoingImageFile already gives an outgoing
// attachment), or srcPath cannot be read at all, and dstPath is never
// written in any of those cases.
func ReencodeImageFile(srcPath, dstPath string, profile imagesafe.Profile) (format imagesafe.OutputFormat, width, height int, err error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}
	if info.Size() > profile.MaxInputLen {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", ErrMediaTooLarge)
	}

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
