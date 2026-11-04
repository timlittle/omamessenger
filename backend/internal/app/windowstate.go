package app

import "context"

// WindowOpen reports whether the window was last left open rather than
// deliberately closed, so a reload can reopen it.
func (c *Commands) WindowOpen(ctx context.Context) (bool, error) {
	return c.store.WindowOpen(ctx)
}

// SetWindowOpen records whether the window is currently open.
func (c *Commands) SetWindowOpen(ctx context.Context, open bool) error {
	return c.store.SetWindowOpen(ctx, open)
}
