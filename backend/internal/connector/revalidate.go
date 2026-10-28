package connector

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// imageSniffLen is how many leading bytes RevalidateDownloadedImage
// and RevalidateOutgoingImage peek at to decide whether a file looks
// like an image at all, before ever reading the rest of it: enough
// for the longest magic number imagesafe.LooksLikeImage checks (a
// WebP's 12-byte RIFF/WEBP header), with a little room to spare.
const imageSniffLen = 16

// RevalidateDownloadedImage reports whether path, a file already in
// the downloaded media cache from a run of this helper that may
// predate one of imagesafe's own checks, should still be kept there
// as is. A file that does not look like an image at all - a video,
// voice note or document, which this cache also holds - is left
// completely untouched and reported true: it was never going to
// reach Qt's own image decoder in the first place, and every one of
// those kinds already gets its own kind-specific safety check on
// every single fetch regardless of age (see SanitizeIfGIF,
// VideoPlayable, AudioPlayable and app.MediaRisk), so revalidating it
// here as well would only risk rejecting a legitimate file this
// package has no way to check at all. A file that does look like an
// image is run through imagesafe.Sanitize under whichever profile a
// fresh download of it would use today - AnimatedGIF for a real GIF
// file, FullImage otherwise, the same choice fetchImage itself makes -
// and kept, rewritten in place when sanitizing changes it, or
// reported false, for the caller to remove, when it fails outright:
// exactly the vulnerability this sweep exists to close (see
// docs/decisions.md). A read failure removes path too, rather than
// ever leaving a file this sweep could not even look at trusted for
// another run.
func RevalidateDownloadedImage(path string) (keep bool, err error) {
	head, size, err := peekFile(path, imageSniffLen)
	if err != nil {
		return false, fmt.Errorf("connector: revalidate downloaded image: %w", errors.Join(err, removeIfPresent(path)))
	}
	if !imagesafe.LooksLikeImage(head) {
		return true, nil
	}

	profile := imagesafe.FullImage
	if bytes.HasPrefix(head, gifMagic) {
		profile = imagesafe.AnimatedGIF
	}
	if size > profile.MaxInputLen {
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("connector: revalidate downloaded image: %w", errors.Join(err, removeIfPresent(path)))
	}

	return sanitizeInPlace(path, data, profile)
}

// RevalidateOutgoingImage reports whether path, a file already in
// the outgoing attachment area from a run of this helper that may
// predate SanitizeOutgoingImageFile, should still be kept there as
// is. It mirrors RevalidateDownloadedImage's own treatment of a file
// that does not look like an image at all - left untouched, reported
// true - for exactly the same reason: a video or other document
// attachment already gets its own kind-specific check on every
// fetch, regardless of this area's own age. A file that does look
// like an image is run through SanitizeOutgoingImageFile, the exact
// full-size profile every outgoing attachment is already sanitized
// through today, so a legacy copy is held to the same bar a fresh
// one gets; a failure there means it is reported false, for the
// caller to remove, falling back to re-copying it from the original
// file it was attached from, when there still is one (see
// app.restoreAttachment). A read failure removes path too, the same
// as RevalidateDownloadedImage.
func RevalidateOutgoingImage(path string) (keep bool, err error) {
	head, _, err := peekFile(path, imageSniffLen)
	if err != nil {
		return false, fmt.Errorf("connector: revalidate outgoing image: %w", errors.Join(err, removeIfPresent(path)))
	}
	if !imagesafe.LooksLikeImage(head) {
		return true, nil
	}

	switch err := SanitizeOutgoingImageFile(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrUnsafeImage), errors.Is(err, ErrMediaTooLarge):
		return false, nil
	default:
		return false, fmt.Errorf("connector: revalidate outgoing image: %w", err)
	}
}

// sanitizeInPlace runs data through imagesafe.Sanitize under profile,
// rewriting path with the sanitized bytes when they differ from data,
// for RevalidateDownloadedImage's own use once it already has data in
// hand and does not need SanitizeOutgoingImageFile's own file-opening
// and size-checking a second time.
func sanitizeInPlace(path string, data []byte, profile imagesafe.Profile) (keep bool, err error) {
	safe, ok := imagesafe.Sanitize(data, profile)
	if !ok {
		return false, nil
	}
	if bytes.Equal(safe, data) {
		return true, nil
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil { //nolint:gosec // deliberate: path is the media cache path this sweep is already revalidating, never built from untrusted input
		return false, fmt.Errorf("connector: revalidate downloaded image: %w", err)
	}

	return true, nil
}
