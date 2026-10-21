package store

import (
	"context"
)

// SeenWhatsNewVersion returns the version the user last saw the in-app
// "What's new" list for, or "" before they have seen any (including a
// fresh install).
func (s *Store) SeenWhatsNewVersion(ctx context.Context) (string, error) {
	var version string

	err := s.db.QueryRowContext(ctx, `SELECT seen_whats_new_version FROM ui_state WHERE id = 1`).Scan(&version)

	return version, wrap("seen whats new version", err)
}

// SetSeenWhatsNewVersion records version as the one the user has now seen
// the "What's new" list for, so the update hint does not show again for
// it.
func (s *Store) SetSeenWhatsNewVersion(ctx context.Context, version string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ui_state SET seen_whats_new_version = ? WHERE id = 1`, version)

	return wrap("set seen whats new version", err)
}
