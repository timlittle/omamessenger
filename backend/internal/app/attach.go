package app

import (
	"context"
	"fmt"
)

// AttachedFile describes a file AttachFile copied into the outgoing
// media area and, when it looked like a photo, sanitized with
// imagesafe, ready for the composer to hand back to Send as its next
// message's attachment. Send reuses Path by moving it into the
// message's own copy, never copying or sanitizing it again (see
// prepareAttachment). Kind is "photo", "video" or "file", sniffed from
// the file's own bytes, never guessed from its name; the composer
// shows a preview only when it is "photo", the one kind imagesafe has
// already checked.
type AttachedFile struct {
	Path   string
	Name   string
	Kind   string
	Width  int
	Height int
}

// AttachFile copies path, a file the user picked from disk, into the
// outgoing media area before the composer ever shows a preview built
// from it: a crafted file attached from disk gets exactly the
// treatment a downloaded or pasted photo already does, run through
// imagesafe before anything decodes it, rather than waiting until the
// message is actually sent (see docs/decisions.md). An oversized or
// unsafe image is refused outright with the same safe message Send's
// own attachment handling already gives a raw disk path.
func (c *Commands) AttachFile(ctx context.Context, path string) (AttachedFile, error) {
	if path == "" {
		return AttachedFile{}, fmt.Errorf("%w: a file is required", ErrInvalidInput)
	}

	media, err := c.prepareAttachment(ctx, newAttachmentID(), path, "")
	if err != nil {
		return AttachedFile{}, err
	}

	return AttachedFile{
		Path: media.Path, Name: media.FileName, Kind: media.Kind,
		Width: media.Width, Height: media.Height,
	}, nil
}
