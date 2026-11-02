package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// clipboardMethods are the media.paste and clipboard.readText entries
// of the protocol's method table - both the clipboard's own one real
// resource, read through the helper's bounded wl-paste - split out of
// methods() itself to keep that function within this codebase's line
// limit.
func clipboardMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"media.paste":        bind(mediaPaste(c)),
		"clipboard.readText": bind(clipboardReadText(c)),
	}
}

// pasteResult is the image media.paste found on the clipboard, copied
// into the outgoing media area.
type pasteResult struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// mediaPaste copies an image off the clipboard, for the composer to
// attach to the next message sent, or fails with an invalid-input error
// when the clipboard holds no image.
func mediaPaste(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		img, err := c.PasteImage(ctx)

		return pasteResult{Path: img.Path, Kind: img.Kind, Width: img.Width, Height: img.Height}, err
	}
}

// clipboardReadTextResult is clipboard.readText's result.
type clipboardReadTextResult struct {
	Text string `json:"text"`
}

// clipboardReadText reads the clipboard's text/plain offer through the
// helper's own bounded wl-paste path, for the composer's Ctrl+V to
// insert at the cursor: the one safe way to read clipboard text at
// all, since Qt's own in-process clipboard reader has no size or time
// limit of its own (see docs/decisions.md). Fails with an
// invalid-input error when the clipboard holds no text.
func clipboardReadText(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		text, err := c.PasteText(ctx)

		return clipboardReadTextResult{Text: text}, err
	}
}
