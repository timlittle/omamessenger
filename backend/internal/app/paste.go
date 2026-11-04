package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errNoClipboardImage reports a paste asked for when the clipboard holds
// no image the helper can read.
var errNoClipboardImage = errors.New("there is no image on the clipboard")

// errUnsafeClipboardImage reports a pasted image that failed the same
// safety check a downloaded photo must pass: the clipboard belongs to
// whichever other program last put something on it, so it is never
// trusted further than a sender's own attachment.
var errUnsafeClipboardImage = errors.New("the pasted image could not be used")

// errNoClipboardText reports a paste asked for when the clipboard
// holds no text the helper can read.
var errNoClipboardText = errors.New("there is no text on the clipboard")

// pasteImageTypes are the clipboard MIME types PasteImage accepts, most
// preferred first.
var pasteImageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp"}

// PastedImage describes an image PasteImage copied from the clipboard.
type PastedImage struct {
	Path   string
	Kind   string
	Width  int
	Height int
}

// PasteImage copies an image off the clipboard into the outgoing media
// area, for the composer to attach to the next message sent.
func (c *Commands) PasteImage(ctx context.Context) (PastedImage, error) {
	if c.clipboard == nil || c.outgoing == nil {
		return PastedImage{}, fmt.Errorf("%w: %w", ErrInvalidInput, errNoClipboardImage)
	}

	mimeType, err := c.clipboardImageType(ctx)
	if err != nil {
		return PastedImage{}, err
	}

	var data bytes.Buffer
	if err := c.clipboard.Read(ctx, mimeType, &data); err != nil {
		return PastedImage{}, fmt.Errorf("clipboard: %w", err)
	}

	safe, err := c.sanitizeOutgoingImage(ctx, data.Bytes())
	if err != nil {
		return PastedImage{}, fmt.Errorf("%w: %w", ErrInvalidInput, errUnsafeClipboardImage)
	}

	id := newAttachmentID()
	name := "pasted." + strings.TrimPrefix(mimeType, "image/")
	stored, err := c.outgoing.Store(ctx, id, name, bytes.NewReader(safe))
	if err != nil {
		return PastedImage{}, fmt.Errorf("clipboard: %w", err)
	}

	width, height := imageDimensions(stored)

	return PastedImage{Path: stored, Kind: domain.MediaPhoto, Width: width, Height: height}, nil
}

// PasteText returns the clipboard's text/plain offer, read through the
// same bounded wl-paste path PasteImage uses, rather than Qt's
// unbounded clipboard reader (see docs/decisions.md). Fails with an
// invalid-input error when the clipboard holds no text, or an
// ordinary error when the bounded read itself was refused.
func (c *Commands) PasteText(ctx context.Context) (string, error) {
	if c.clipboard == nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidInput, errNoClipboardText)
	}

	text, err := c.clipboard.ReadText(ctx)
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	if text == "" {
		return "", fmt.Errorf("%w: %w", ErrInvalidInput, errNoClipboardText)
	}

	return text, nil
}

// clipboardImageType asks the clipboard what it offers and returns the
// first type PasteImage knows how to read, or an invalid-input error
// when none of them is there.
func (c *Commands) clipboardImageType(ctx context.Context) (string, error) {
	offered, err := c.clipboard.Types(ctx)
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}

	for _, want := range pasteImageTypes {
		if slices.Contains(offered, want) {
			return want, nil
		}
	}

	return "", fmt.Errorf("%w: %w", ErrInvalidInput, errNoClipboardImage)
}
