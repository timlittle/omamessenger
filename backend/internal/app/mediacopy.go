package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// MaxCopyBytes bounds how large a file media.copy will put on the
// system clipboard. A clipboard offer sits in the Wayland compositor,
// read by whichever program pastes next, for as long as it stands -
// unlike a download, which this helper's own cache already bounds -
// so this is a backstop independent of imagesafe's or transcode's own
// limits, not merely a restatement of them. 64 MiB comfortably covers
// any photo (imagesafe.FullImage's own 32 MiB input cap) or a short
// video clip, while staying far below the 2 GiB a deliberate fetch
// may otherwise download.
const MaxCopyBytes = 64 << 20

// errNoClipboard reports a media.copy asked for before this run wired
// up a clipboard at all.
var errNoClipboard = fmt.Errorf("%w: copying media is not available", ErrInvalidInput)

// errUnsafeMediaCopy reports that the message's own downloaded bytes
// do not match what copyableContentType allows for its kind: a
// contact's modified client can claim any kind it likes for a
// message, so the bytes actually copied must be checked the same way
// every other download already is, never trusted just because a
// message claims to carry them (see docs/decisions.md).
var errUnsafeMediaCopy = fmt.Errorf("%w: this media cannot be copied", ErrInvalidInput)

// errMediaCopyTooLarge reports that a file passed every other check
// but is over MaxCopyBytes.
var errMediaCopyTooLarge = fmt.Errorf("%w: this file is too large to copy", ErrInvalidInput)

// CopyMedia puts a message's photo, sticker or video onto the system
// clipboard, through clipboard (wl-copy), fetching it first if it is
// not already on this machine. It copies only this helper's own
// output, never a sender's bytes directly: a photo or sticker's
// display copy, already what FetchMedia always returns for those
// kinds, or a video's play copy, built on demand here the same way a
// play fetch builds one for the UI's own player (see
// docs/decisions.md). mimeType is always sniffed fresh from those
// bytes (see sniffContentType), never a sender's own claimed kind or
// file name, and copyableContentType refuses to copy anything whose
// sniffed bytes do not actually match the kind that sniff is checked
// against, so a modified client cannot put arbitrary bytes on the
// clipboard merely by claiming the message carries one of the kinds
// this ever copies.
func (c *Commands) CopyMedia(ctx context.Context, messageID string) error {
	if c.clipboard == nil {
		return errNoClipboard
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return err
	}

	path, playPath, _, _, _, _, err := c.FetchMedia(ctx, messageID, false, true)
	if err != nil {
		return err
	}

	source, ok := copySource(m, path, playPath)
	if !ok {
		return errUnsafeMediaCopy
	}

	contentType, err := copyableMediaType(m, source)
	if err != nil {
		return err
	}

	return c.writeMediaToClipboard(ctx, source, contentType)
}

// copySource is the one file CopyMedia ever reads for m: path, the
// display copy FetchMedia already returns for a photo or sticker, or
// playPath, the video play copy a play fetch just built or reused
// (see docs/decisions.md). ok is false whenever there is no such
// file to copy - any other media kind, or a video whose play copy
// could not be built at all - never a reason to fall back to the
// original download.
func copySource(m domain.Message, path, playPath string) (string, bool) {
	if m.Media == nil {
		return "", false
	}

	switch m.Media.Kind {
	case domain.MediaPhoto, domain.MediaSticker:
		return path, true
	case domain.MediaVideo:
		return playPath, playPath != ""
	default:
		return "", false
	}
}

// copyableMediaType sniffs source's own bytes and confirms they are
// safe to copy for m's own kind (see copyableContentType), returning
// the sniffed MIME type to copy under. err is errUnsafeMediaCopy when
// the content does not match, never a bare "false" a caller could
// mistake for some other failure.
func copyableMediaType(m domain.Message, source string) (string, error) {
	contentType, err := sniffContentType(source)
	if err != nil {
		return "", fmt.Errorf("media: copy: %w", err)
	}

	if !copyableContentType(m.Media.Kind, contentType) {
		return "", errUnsafeMediaCopy
	}

	return contentType, nil
}

// writeMediaToClipboard puts path's own bytes on the clipboard under
// contentType, refusing anything over MaxCopyBytes before ever
// opening the file to read it.
func (c *Commands) writeMediaToClipboard(ctx context.Context, path, contentType string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("media: copy: %w", err)
	}
	if info.Size() > MaxCopyBytes {
		return errMediaCopyTooLarge
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("media: copy: %w", err)
	}
	defer f.Close() // reading only; nothing to lose on a close error

	if err := c.clipboard.Write(ctx, contentType, f); err != nil {
		return fmt.Errorf("media: copy: %w", err)
	}

	return nil
}

// copyableContentType reports whether contentType, sniffed fresh from
// source's own bytes (copySource's display or play copy, never a raw
// download), is safe to put on the clipboard for kind: an image for a
// photo or sticker, a video for a video - every kind this UI's own
// copy action ever offers (see MediaController.qml's copyMedia and
// Media.mimeFor) but checked again here, since that gate is the UI's
// own convenience, never the security boundary - a future UI change,
// or a client calling media.copy directly, must not be able to copy a
// kind this never allows just because some message happens to carry
// it. Every other kind, including a plain file or a voice note, is
// refused outright: neither one has an obvious MIME family of its own
// to check sniffed bytes against the way a photo or video does. A
// video's own content always sniffs as video/*: copySource only ever
// hands this a play copy the sandboxed transcoder itself produced,
// never raw, sender-controlled bytes that could sniff as anything
// else (see docs/decisions.md).
func copyableContentType(kind, contentType string) bool {
	switch kind {
	case domain.MediaPhoto, domain.MediaSticker:
		return strings.HasPrefix(contentType, "image/")
	case domain.MediaVideo:
		return strings.HasPrefix(contentType, "video/")
	default:
		return false
	}
}
