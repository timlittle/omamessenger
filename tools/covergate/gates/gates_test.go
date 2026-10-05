package gates

import "testing"

func TestFor(t *testing.T) {
	cases := map[string]struct {
		min float64
		ok  bool
	}{
		"backend/internal/domain": {100, true},
		"backend/internal/notify": {75, true},
		"backend":                 {80, true},
		"tools/omalint/size":      {ToolsMin, true},
		"backend/internal/x":      {0, false},
	}
	for pkg, want := range cases {
		if min, ok := For(pkg); min != want.min || ok != want.ok {
			t.Errorf("For(%q) = %v, %t; want %v, %t", pkg, min, ok, want.min, want.ok)
		}
	}
}
