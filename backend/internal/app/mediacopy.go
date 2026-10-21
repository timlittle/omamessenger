package app

import (
	"context"
	"fmt"
	"os"
)

// errNoClipboard reports a media.copy asked for before this run wired
// up a clipboard at all.
var errNoClipboard = fmt.Errorf("%w: copying media is not available", ErrInvalidInput)

// CopyMedia puts a message's photo, video, file or voice note onto the
// system clipboard, through clipboard (wl-copy), fetching it first if
// it is not already on this machine. mimeType is always sniffed fresh
// from the downloaded bytes (see sniffContentType), never a sender's
// own claimed kind or file name, the same rule fetchIntoCache already
// applies to a cached file's own name: this is what let a chat name or
// file name reach a shell command when the UI used to build this call
// itself (see ui/controllers/MediaController.qml and
// docs/decisions.md), and sniffing here instead of trusting the
// message removes the one piece of sender-controlled text this path
// still carried.
func (c *Commands) CopyMedia(ctx context.Context, messageID string) error {
	if c.clipboard == nil {
		return errNoClipboard
	}

	path, err := c.FetchMedia(ctx, messageID, false)
	if err != nil {
		return err
	}

	contentType, err := sniffContentType(path)
	if err != nil {
		return fmt.Errorf("media: copy: %w", err)
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
