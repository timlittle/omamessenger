package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// attachPathParams names a file the user picked from disk for
// media.attach to copy into the outgoing media area. AccountID selects
// the connector's own attachment limit; "" falls back to
// app.MaxAttachmentSize.
type attachPathParams struct {
	Path      string `json:"path"`
	AccountID string `json:"accountId"`
}

// attachResult is the file media.attach copied into the outgoing media
// area and, for a photo, sanitized with imagesafe. Name is the
// original file name, since Path's base name carries an internal id.
// Size and Limit let the UI warn when the file sits close to the cap.
type attachResult struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
	Limit  int64  `json:"limit"`
}

// mediaAttach copies a file the user picked from disk into the
// outgoing media area, sanitizing it with imagesafe when it looks like
// a photo, before the composer ever shows a preview built from it; an
// oversized or unsafe image fails with an invalid-input error instead.
func mediaAttach(c *app.Commands) func(context.Context, attachPathParams) (any, error) {
	return func(ctx context.Context, p attachPathParams) (any, error) {
		f, err := c.AttachFile(ctx, p.Path, p.AccountID)

		return attachResult{Path: f.Path, Name: f.Name, Kind: f.Kind, Width: f.Width, Height: f.Height, Size: f.Size, Limit: f.Limit}, err
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
