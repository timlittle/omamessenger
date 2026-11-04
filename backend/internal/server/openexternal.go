package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// openExternalMethods is links.open's and files.open's own entry. Both
// open something outside this process through the desktop portal
// instead of xdg-open, so the target never lands in a process argv
// another local user could read (see docs/decisions.md).
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

// linksOpen opens url through the desktop portal, once it has passed
// the scheme allow-list in app.Commands.OpenLink.
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

// filesOpen opens the local file at path through the desktop portal.
func filesOpen(c *app.Commands) func(context.Context, fileOpenParams) (any, error) {
	return func(ctx context.Context, p fileOpenParams) (any, error) {
		return struct{}{}, c.OpenFile(ctx, p.Path)
	}
}
