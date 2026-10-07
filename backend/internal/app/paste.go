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
		return PastedImage{}, fmt.Errorf("%w: %s", ErrInvalidInput, errNoClipboardImage)
	}

	mimeType, err := c.clipboardImageType(ctx)
	if err != nil {
		return PastedImage{}, err
	}

	var data bytes.Buffer
	if err := c.clipboard.Read(ctx, mimeType, &data); err != nil {
		return PastedImage{}, fmt.Errorf("clipboard: %w", err)
	}

	id := newAttachmentID()
	name := "pasted." + strings.TrimPrefix(mimeType, "image/")
	stored, err := c.outgoing.Store(ctx, id, name, &data)
	if err != nil {
		return PastedImage{}, fmt.Errorf("clipboard: %w", err)
	}

	width, height := imageDimensions(stored)

	return PastedImage{Path: stored, Kind: domain.MediaPhoto, Width: width, Height: height}, nil
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

	return "", fmt.Errorf("%w: %s", ErrInvalidInput, errNoClipboardImage)
}
