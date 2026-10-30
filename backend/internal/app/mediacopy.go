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
// so this is a backstop independent of imagesafe's or videosafe's own
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
// not already on this machine. mimeType is always sniffed fresh from
// the downloaded bytes (see sniffContentType), never a sender's own
// claimed kind or file name - the same rule fetchIntoCache already
// applies to a cached file's own name - and copyableContentType
// refuses to copy anything whose sniffed bytes do not actually match
// the kind that sniff is checked against, so a modified client cannot
// put arbitrary bytes on the clipboard merely by claiming the message
// carries one of the kinds this ever copies (see docs/decisions.md).
func (c *Commands) CopyMedia(ctx context.Context, messageID string) error {
	if c.clipboard == nil {
		return errNoClipboard
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return err
	}

	path, err := c.FetchMedia(ctx, messageID, false)
	if err != nil {
		return err
	}

	contentType, err := c.copyableMediaType(ctx, m, path)
	if err != nil {
		return err
	}

	return c.writeMediaToClipboard(ctx, path, contentType)
}

// copyableMediaType sniffs path's own downloaded bytes and confirms
// they are safe to copy for m's own kind (see copyableContentType),
// returning the sniffed MIME type to copy under. err is
// errUnsafeMediaCopy when the content does not match, never a bare
// "false" a caller could mistake for some other failure.
func (c *Commands) copyableMediaType(ctx context.Context, m domain.Message, path string) (string, error) {
	contentType, err := sniffContentType(path)
	if err != nil {
		return "", fmt.Errorf("media: copy: %w", err)
	}

	allowed, err := c.copyableContentType(ctx, m, path, contentType)
	if err != nil {
		return "", err
	}
	if !allowed {
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
// path's own downloaded bytes, is safe to put on the clipboard for
// m's own media kind: an image for a photo or sticker, every kind
// this UI's own copy action ever offers (see MediaController.qml's
// copyMedia and Media.mimeFor) but checked again here, since that
// gate is the UI's own convenience, never the security boundary - a
// future UI change, or a client calling media.copy directly, must not
// be able to copy a kind this never allows just because some message
// happens to carry it. Every other kind, including a plain file or a
// voice note, is refused outright: neither one has an obvious MIME
// family of its own to check sniffed bytes against the way a photo or
// video does.
func (c *Commands) copyableContentType(ctx context.Context, m domain.Message, path, contentType string) (bool, error) {
	if m.Media == nil {
		return false, nil
	}

	switch m.Media.Kind {
	case domain.MediaPhoto, domain.MediaSticker:
		return strings.HasPrefix(contentType, "image/"), nil
	case domain.MediaVideo:
		return c.copyableVideo(ctx, m.ID, path, contentType)
	default:
		return false, nil
	}
}

// copyableVideo reports whether a video message's own downloaded
// bytes may be copied: a video a sending service marked as a silent
// "GIF" that genuinely sanitizes as one (see DecodeAsForMedia) must
// itself still sniff as an image, since that is what its bytes
// actually are by this point; an ordinary video must both sniff as a
// video and pass VideoPlayableForMedia's own check - the same verdict
// the UI already shows a video through before ever decoding it in
// place, never the sender's own claim that this is a playable video
// at all.
func (c *Commands) copyableVideo(ctx context.Context, messageID, path, contentType string) (bool, error) {
	decodeAs, err := c.DecodeAsForMedia(ctx, messageID, path)
	if err != nil {
		return false, err
	}
	if decodeAs == "gif" {
		return strings.HasPrefix(contentType, "image/"), nil
	}
	if !strings.HasPrefix(contentType, "video/") {
		return false, nil
	}

	return c.VideoPlayableForMedia(ctx, messageID, path)
}
