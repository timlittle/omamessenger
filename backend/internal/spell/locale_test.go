package spell

import "testing"

// TestChooseLocale checks which bundled dictionary a setting and a locale
// environment resolve to.
func TestChooseLocale(t *testing.T) {
	t.Parallel()

	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}

	cases := []struct {
		name    string
		setting string
		env     func(string) string
		want    string
	}{
		{"explicit en_GB wins over locale", "en_GB", env(map[string]string{"LANG": "en_US.UTF-8"}), "en_GB"},
		{"explicit en_US wins over locale", "en_US", env(map[string]string{"LANG": "en_GB.UTF-8"}), "en_US"},
		{"auto with en_GB locale", "auto", env(map[string]string{"LANG": "en_GB.UTF-8"}), "en_GB"},
		{"auto with en_AU locale", "auto", env(map[string]string{"LANG": "en_AU.UTF-8"}), "en_GB"},
		{"auto with en_NZ locale", "auto", env(map[string]string{"LANG": "en_NZ.UTF-8"}), "en_GB"},
		{"auto with en_IE locale", "auto", env(map[string]string{"LANG": "en_IE.UTF-8"}), "en_GB"},
		{"auto with en_US locale", "auto", env(map[string]string{"LANG": "en_US.UTF-8"}), "en_US"},
		{"auto with no locale set", "auto", env(nil), "en_US"},
		{"LC_MESSAGES wins over LANG", "auto", env(map[string]string{"LANG": "en_US.UTF-8", "LC_MESSAGES": "en_GB.UTF-8"}), "en_GB"},
		{"unknown setting falls back to auto", "fr_FR", env(map[string]string{"LANG": "en_GB.UTF-8"}), "en_GB"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := chooseLocale(c.setting, c.env); got != c.want {
				t.Errorf("chooseLocale(%q) = %q, want %q", c.setting, got, c.want)
			}
		})
	}
}
