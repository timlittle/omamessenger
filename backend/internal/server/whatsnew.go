package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// whatsNewVersionResult carries the version string the UI compares
// against its own running version to decide whether to show the
// one-time update hint; empty before anything has been recorded, which
// a fresh install's own version never is.
type whatsNewVersionResult struct {
	Version string `json:"version"`
}

// whatsNewGetSeenVersion returns the version the user last saw the
// in-app "What's new" list for.
func whatsNewGetSeenVersion(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		version, err := c.SeenWhatsNewVersion(ctx)
		return whatsNewVersionResult{Version: version}, err
	}
}

// whatsNewSetSeenVersion records version as the one the user has now
// seen the "What's new" list for.
func whatsNewSetSeenVersion(c *app.Commands) func(context.Context, whatsNewVersionResult) (any, error) {
	return func(ctx context.Context, p whatsNewVersionResult) (any, error) {
		return none{}, c.SetSeenWhatsNewVersion(ctx, p.Version)
	}
}
