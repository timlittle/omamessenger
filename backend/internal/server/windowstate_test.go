package server_test

import "testing"

// TestMethods_WindowOpen confirms getWindowOpen starts false for a fresh
// install and reads back whatever setWindowOpen last recorded.
func TestMethods_WindowOpen(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	open, err := call[struct {
		Open bool `json:"open"`
	}](t, s, "ui.getWindowOpen", nil)
	if err != nil || open.Open {
		t.Errorf("ui.getWindowOpen = %+v, %v, want false, nil", open, err)
	}

	if _, err := call[struct{}](t, s, "ui.setWindowOpen", map[string]bool{"open": true}); err != nil {
		t.Errorf("ui.setWindowOpen(true) = %v", err)
	}

	open, err = call[struct {
		Open bool `json:"open"`
	}](t, s, "ui.getWindowOpen", nil)
	if err != nil || !open.Open {
		t.Errorf("ui.getWindowOpen after set = %+v, %v, want true, nil", open, err)
	}

	if _, err := call[struct{}](t, s, "ui.setWindowOpen", map[string]bool{"open": false}); err != nil {
		t.Errorf("ui.setWindowOpen(false) = %v", err)
	}

	open, err = call[struct {
		Open bool `json:"open"`
	}](t, s, "ui.getWindowOpen", nil)
	if err != nil || open.Open {
		t.Errorf("ui.getWindowOpen after unset = %+v, %v, want false, nil", open, err)
	}
}
