package store_test

import (
	"testing"
)

// TestSeenWhatsNewVersion_EmptyBeforeAnySet confirms a fresh database
// reports no seen version, so a fresh install's one-time hint logic can
// tell it apart from a version it has already recorded.
func TestSeenWhatsNewVersion_EmptyBeforeAnySet(t *testing.T) {
	t.Parallel()

	s := openStore(t)

	version, err := s.SeenWhatsNewVersion(t.Context())
	if err != nil || version != "" {
		t.Errorf("SeenWhatsNewVersion = %q, %v, want \"\", nil", version, err)
	}
}

// TestSetSeenWhatsNewVersion_RoundTrips confirms a recorded version is
// read back, and that setting it again replaces rather than duplicates
// it.
func TestSetSeenWhatsNewVersion_RoundTrips(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()

	if err := s.SetSeenWhatsNewVersion(ctx, "0.3.5"); err != nil {
		t.Fatalf("SetSeenWhatsNewVersion: %v", err)
	}

	version, err := s.SeenWhatsNewVersion(ctx)
	if err != nil || version != "0.3.5" {
		t.Errorf("SeenWhatsNewVersion = %q, %v, want \"0.3.5\", nil", version, err)
	}

	if err := s.SetSeenWhatsNewVersion(ctx, "0.3.6"); err != nil {
		t.Fatalf("SetSeenWhatsNewVersion: %v", err)
	}

	version, err = s.SeenWhatsNewVersion(ctx)
	if err != nil || version != "0.3.6" {
		t.Errorf("SeenWhatsNewVersion = %q, %v, want \"0.3.6\", nil", version, err)
	}
}
