package app_test

import "testing"

// TestSeenWhatsNewVersion_RoundTrips confirms Commands forwards both
// calls to the store unchanged, starting empty for a fresh install.
func TestSeenWhatsNewVersion_RoundTrips(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	version, err := f.commands.SeenWhatsNewVersion(ctx)
	if err != nil || version != "" {
		t.Errorf("SeenWhatsNewVersion = %q, %v, want \"\", nil", version, err)
	}

	if err := f.commands.SetSeenWhatsNewVersion(ctx, "0.3.5"); err != nil {
		t.Fatalf("SetSeenWhatsNewVersion: %v", err)
	}

	version, err = f.commands.SeenWhatsNewVersion(ctx)
	if err != nil || version != "0.3.5" {
		t.Errorf("SeenWhatsNewVersion = %q, %v, want \"0.3.5\", nil", version, err)
	}
}
