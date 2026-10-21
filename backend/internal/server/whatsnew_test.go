package server_test

import "testing"

// TestMethods_WhatsNewSeenVersion confirms getSeenVersion starts empty
// for a fresh install and reads back whatever setSeenVersion last
// recorded.
func TestMethods_WhatsNewSeenVersion(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	seen, err := call[struct {
		Version string `json:"version"`
	}](t, s, "whatsNew.getSeenVersion", nil)
	if err != nil || seen.Version != "" {
		t.Errorf("whatsNew.getSeenVersion = %+v, %v, want empty version, nil", seen, err)
	}

	if _, err := call[struct{}](t, s, "whatsNew.setSeenVersion", map[string]string{"version": "0.3.5"}); err != nil {
		t.Errorf("whatsNew.setSeenVersion = %v", err)
	}

	seen, err = call[struct {
		Version string `json:"version"`
	}](t, s, "whatsNew.getSeenVersion", nil)
	if err != nil || seen.Version != "0.3.5" {
		t.Errorf("whatsNew.getSeenVersion after set = %+v, %v, want \"0.3.5\", nil", seen, err)
	}
}
