package app_test

import "testing"

// TestWindowOpen_RoundTrips confirms Commands forwards both calls to the
// store unchanged, starting false for a fresh install.
func TestWindowOpen_RoundTrips(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	open, err := f.commands.WindowOpen(ctx)
	if err != nil || open {
		t.Errorf("WindowOpen = %v, %v, want false, nil", open, err)
	}

	if err := f.commands.SetWindowOpen(ctx, true); err != nil {
		t.Fatalf("SetWindowOpen: %v", err)
	}

	open, err = f.commands.WindowOpen(ctx)
	if err != nil || !open {
		t.Errorf("WindowOpen = %v, %v, want true, nil", open, err)
	}

	if err := f.commands.SetWindowOpen(ctx, false); err != nil {
		t.Fatalf("SetWindowOpen: %v", err)
	}

	open, err = f.commands.WindowOpen(ctx)
	if err != nil || open {
		t.Errorf("WindowOpen after closing = %v, %v, want false, nil", open, err)
	}
}
