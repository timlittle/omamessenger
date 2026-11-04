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

// WindowOpen reports whether the window was last left open rather
// than deliberately closed, false before anything has been recorded.
func (s *Store) WindowOpen(ctx context.Context) (bool, error) {
	var open bool

	err := s.db.QueryRowContext(ctx, `SELECT window_open FROM ui_state WHERE id = 1`).Scan(&open)

	return open, wrap("window open", err)
}

// SetWindowOpen records whether the window is currently open: true when
// it is shown, false only when the user deliberately closes it. See
// docs/decisions.md for how a deliberate close is told apart from the
// window being torn down.
func (s *Store) SetWindowOpen(ctx context.Context, open bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ui_state SET window_open = ? WHERE id = 1`, open)

	return wrap("set window open", err)
}
