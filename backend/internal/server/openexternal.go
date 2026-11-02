package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// openExternalMethods is links.open's and files.open's own entry, kept
// in a function of its own rather than growing methods' table literal
// further. Both open something outside this process through the
// desktop portal, over D-Bus, instead of starting xdg-open with the
// target in its own argv, where any other local user could read it
// through /proc (see docs/decisions.md).
func openExternalMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"links.open": bind(linksOpen(c)),
		"files.open": bind(filesOpen(c)),
	}
}

// linkOpenParams is the link to open, exactly as a message, a link
// preview or a service's own text names it.
type linkOpenParams struct {
	URL string `json:"url"`
}

// linksOpen opens url through the desktop portal, once url has passed
// the same scheme allow-list the UI already checked (see
// app.Commands.OpenLink): the method WindowController.openExternalLink
// calls instead of Qt.openUrlExternally.
func linksOpen(c *app.Commands) func(context.Context, linkOpenParams) (any, error) {
	return func(ctx context.Context, p linkOpenParams) (any, error) {
		return struct{}{}, c.OpenLink(ctx, p.URL)
	}
}

// fileOpenParams is the local path to open: already produced by this
// helper (a message's downloaded photo, video or file) or owned by the
// user, such as this plugin's own keys.conf.
type fileOpenParams struct {
	Path string `json:"path"`
}

// filesOpen opens path through the desktop portal: the method
// MediaController's and ConversationController's default
// openExternally, and Service.qml's openKeyConfigFile, call instead of
// Qt.openUrlExternally.
func filesOpen(c *app.Commands) func(context.Context, fileOpenParams) (any, error) {
	return func(ctx context.Context, p fileOpenParams) (any, error) {
		return struct{}{}, c.OpenFile(ctx, p.Path)
	}
}
