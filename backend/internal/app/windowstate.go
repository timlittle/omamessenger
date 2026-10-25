package app

import "context"

// WindowOpen reports whether the UI's own window was last left open
// rather than deliberately closed, so the UI knows whether to reopen it
// after a shell restart or plugin reload destroyed it without the user
// asking to close it.
func (c *Commands) WindowOpen(ctx context.Context) (bool, error) {
	return c.store.WindowOpen(ctx)
}

// SetWindowOpen records whether the window is currently open.
func (c *Commands) SetWindowOpen(ctx context.Context, open bool) error {
	return c.store.SetWindowOpen(ctx, open)
}
