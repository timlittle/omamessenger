package domain_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAllowedLinkURL_OnlyAllowsTheKnownSchemes guards the one allow-list
// every connector's link span and link preview check before ever
// reporting a URL the UI will open with no filter of its own.
func TestAllowedLinkURL_OnlyAllowsTheKnownSchemes(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"http://example.com":      true,
		"https://example.com":     true,
		"mailto:a@example.com":    true,
		"tg://resolve?domain=x":   true,
		"HTTPS://EXAMPLE.COM":     true,
		"javascript:alert(1)":     false,
		"file:///etc/passwd":      false,
		"data:text/html,<script>": false,
		"vbscript:msgbox(1)":      false,
		"no scheme at all":        false,
		"":                        false,
	}

	for url, want := range tests {
		if got := domain.AllowedLinkURL(url); got != want {
			t.Errorf("AllowedLinkURL(%q) = %t, want %t", url, got, want)
		}
	}
}
