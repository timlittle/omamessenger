package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// attachPathParams names a file the user picked from disk for
// media.attach to copy into the outgoing media area.
type attachPathParams struct {
	Path string `json:"path"`
}

// attachResult is the file media.attach copied into the outgoing media
// area and, for a photo, sanitized with imagesafe. Name is the
// attachment's original file name, kept separately since Path's own
// base name carries an internal id; the UI sends both back as
// messages.send's attachment once the user sends it (see
// attachmentParams in methods.go).
type attachResult struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// mediaAttach copies a file the user picked from disk into the
// outgoing media area, sanitizing it with imagesafe when it looks like
// a photo, before the composer ever shows a preview built from it; an
// oversized or unsafe image fails with an invalid-input error instead.
func mediaAttach(c *app.Commands) func(context.Context, attachPathParams) (any, error) {
	return func(ctx context.Context, p attachPathParams) (any, error) {
		f, err := c.AttachFile(ctx, p.Path)

		return attachResult{Path: f.Path, Name: f.Name, Kind: f.Kind, Width: f.Width, Height: f.Height}, err
	}
}

// attachmentName is the attachment's display name override a
// messages.send request carries (see attachmentParams in methods.go),
// or "" when it has none.
func attachmentName(a *attachmentParams) string {
	if a == nil {
		return ""
	}

	return a.Name
}
