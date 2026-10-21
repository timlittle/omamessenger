package app

import "context"

// SeenWhatsNewVersion returns the version the UI last told us the user
// saw the in-app "What's new" list for, or "" for a fresh install.
func (c *Commands) SeenWhatsNewVersion(ctx context.Context) (string, error) {
	return c.store.SeenWhatsNewVersion(ctx)
}

// SetSeenWhatsNewVersion records version as the one the user has now seen
// the "What's new" list for.
func (c *Commands) SetSeenWhatsNewVersion(ctx context.Context, version string) error {
	return c.store.SetSeenWhatsNewVersion(ctx, version)
}
