package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// fileMethods is files.pick's own entry, kept in a function of its own
// rather than growing methods' table literal further.
func fileMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"files.pick": bind(filesPick(c)),
	}
}

// filePickParams names what the file picker is for ("attach" or
// "sound", deciding the filters offered) and the dialog's title.
type filePickParams struct {
	Purpose string `json:"purpose"`
	Title   string `json:"title"`
}

// filePickResult is the file the user picked. Path is omitted when the
// user cancelled the dialog, never an error: cancelling is not a
// failure.
type filePickResult struct {
	Path string `json:"path,omitempty"`
}

// filesPick opens a file-choosing dialog through the xdg-desktop-portal
// FileChooser, restricted to the kinds p.Purpose names (see
// docs/decisions.md for why not an in-process dialog).
func filesPick(c *app.Commands) func(context.Context, filePickParams) (any, error) {
	return func(ctx context.Context, p filePickParams) (any, error) {
		path, err := c.PickFile(ctx, p.Purpose, p.Title)

		return filePickResult{Path: path}, err
	}
}
