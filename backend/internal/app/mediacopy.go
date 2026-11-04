package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// MaxCopyBytes bounds how large a file media.copy will put on the
// system clipboard, independent of imagesafe's or transcode's own
// limits: a clipboard offer sits in the compositor for as long as it
// stands, unlike a bounded cached download.
const MaxCopyBytes = 64 << 20

// errNoClipboard reports a media.copy asked for before this run wired
// up a clipboard at all.
var errNoClipboard = fmt.Errorf("%w: copying media is not available", ErrInvalidInput)

// errUnsafeMediaCopy reports that the message's downloaded bytes do
// not match what copyableContentType allows for its claimed kind.
var errUnsafeMediaCopy = fmt.Errorf("%w: this media cannot be copied", ErrInvalidInput)

// errMediaCopyTooLarge reports that a file passed every other check
// but is over MaxCopyBytes.
var errMediaCopyTooLarge = fmt.Errorf("%w: this file is too large to copy", ErrInvalidInput)

// CopyMedia puts a message's photo, sticker or video onto the system
// clipboard via clipboard (wl-copy), fetching it first if needed. It
// copies only this helper's own output (a display or play copy),
// never a sender's bytes directly, and the content type is always
// sniffed fresh rather than trusted from the sender's claim.
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

// copySource is the one file CopyMedia reads for m: the display copy
// for a photo or sticker, or the play copy for a video. ok is false
// when there is no such file; it never falls back to the raw
// download.
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

// copyableMediaType sniffs source's bytes and confirms they are safe
// to copy for m's kind, returning the sniffed MIME type to copy
// under.
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

// writeMediaToClipboard puts path's bytes on the clipboard under
// contentType, refusing anything over MaxCopyBytes before opening
// the file.
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

// copyableContentType reports whether contentType, sniffed fresh
// from the copy source, is safe to put on the clipboard for kind:
// an image for a photo or sticker, a video for a video. This is the
// security boundary, not just the UI's own gate, so every other kind
// (a plain file, a voice note) is refused outright.
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
