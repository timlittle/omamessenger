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

// filesPick opens a file-choosing dialog outside this process (the
// xdg-desktop-portal FileChooser, over D-Bus), restricted to the file
// kinds p.Purpose names; see app.Commands.PickFile. A portal that
// cannot be reached fails with an internal error carrying a safe
// "unavailable" reason (see rpcError), the UI's own cue to fall back
// to a different file picker rather than the in-process GTK dialog
// that once crashed the whole shell (see docs/decisions.md).
func filesPick(c *app.Commands) func(context.Context, filePickParams) (any, error) {
	return func(ctx context.Context, p filePickParams) (any, error) {
		path, err := c.PickFile(ctx, p.Purpose, p.Title)

		return filePickResult{Path: path}, err
	}
}
