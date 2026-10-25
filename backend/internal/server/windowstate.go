package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// windowStateMethods are the ui.getWindowOpen and ui.setWindowOpen
// entries of the protocol's method table, split out of methods() itself
// to keep that function within this codebase's line limit.
func windowStateMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"ui.getWindowOpen": bind(uiGetWindowOpen(c)),
		"ui.setWindowOpen": bind(uiSetWindowOpen(c)),
	}
}

// windowOpenResult carries whether the window was last left open rather
// than deliberately closed; false before anything has been recorded,
// which a fresh install never has.
type windowOpenResult struct {
	Open bool `json:"open"`
}

// uiGetWindowOpen reports whether the window was last left open.
func uiGetWindowOpen(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		open, err := c.WindowOpen(ctx)
		return windowOpenResult{Open: open}, err
	}
}

// uiSetWindowOpen records whether the window is currently open: true
// when it is shown, false only when the user deliberately closes it.
func uiSetWindowOpen(c *app.Commands) func(context.Context, windowOpenResult) (any, error) {
	return func(ctx context.Context, p windowOpenResult) (any, error) {
		return none{}, c.SetWindowOpen(ctx, p.Open)
	}
}
