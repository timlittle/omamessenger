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

// TestAllowedLinkURL_RefusesABackslashAnywhereInTheURL guards a security
// review's own finding: a browser's own WHATWG URL parser ends an
// http(s) authority at a backslash the same as a forward slash, so
// "https://evil.example\@paypal.com/login" opens evil.example while
// the old scheme-only check, and Links.urlHost's own authority split
// (ending only at "/", "?" or "#"), read the real host as
// paypal.com - the part after the backslash. A URL carrying a
// backslash anywhere is refused outright now, whatever else it looks
// like, rather than opened on the strength of a host this helper
// computed differently from the browser that will actually resolve
// it.
func TestAllowedLinkURL_RefusesABackslashAnywhereInTheURL(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a backslash splitting a fake userinfo from the real host": `https://evil.example\@paypal.com/login`,
		"both separators replaced by backslashes":                  `https:\\evil.example`,
		"one of the two separators replaced by a backslash":        `https:/\evil.example`,
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if domain.AllowedLinkURL(url) {
				t.Errorf("AllowedLinkURL(%q) = true, want false", url)
			}
		})
	}
}

// TestAllowedLinkURL_RefusesHTTPWithNoRealAuthority guards the one
// variant a bare backslash check cannot catch on its own:
// "https:evil.example" carries no "//" and no backslash either, yet a
// browser's own URL parser still resolves "evil.example" as its host
// (it tolerates any number of "/" or "\" - including zero - right
// after the scheme's own ":"). Refusing it needs its own rule: an
// http(s) URL must spell out "scheme://" followed by a real,
// non-empty authority.
func TestAllowedLinkURL_RefusesHTTPWithNoRealAuthority(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no separator at all":                "https:evil.example",
		"the same gap for plain http":        "http:evil.example",
		"an empty authority before the path": "https:///evil.example",
		"nothing after the scheme at all":    "https://",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if domain.AllowedLinkURL(url) {
				t.Errorf("AllowedLinkURL(%q) = true, want false", url)
			}
		})
	}
}

// TestAllowedLinkURL_AllowsOrdinaryHTTPAuthorities confirms the new
// authority rule still accepts every shape a real link legitimately
// takes, so TestAllowedLinkURL_RefusesHTTPWithNoRealAuthority's fix
// does not also catch ordinary links in its net.
func TestAllowedLinkURL_AllowsOrdinaryHTTPAuthorities(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a plain host":            "https://example.com",
		"a host with a path":      "https://example.com/path",
		"a host with a port":      "https://example.com:8443/path",
		"userinfo before the @":   "https://user:pass@example.com/path",
		"a query string, no path": "https://example.com?a=1",
		"a bracketed IPv6 host":   "https://[2001:db8::1]/path",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !domain.AllowedLinkURL(url) {
				t.Errorf("AllowedLinkURL(%q) = false, want true", url)
			}
		})
	}
}

// TestAllowedLinkURL_RefusesHiddenOrUnusualCharacters guards the
// general rule behind the specific backslash cases above: a URL
// carrying whitespace, an ASCII control character, or one of the
// Unicode "format" characters (category Cf) a spoofed link has used
// to hide or split its own destination - zero-width spaces and marks,
// bidi embeddings, overrides and isolates, word joiner and invisible
// operators, or the byte-order mark - is refused outright, the same
// as a backslash.
func TestAllowedLinkURL_RefusesHiddenOrUnusualCharacters(t *testing.T) {
	t.Parallel()

	zeroWidthSpace := string(rune(8203))
	rightToLeftMark := string(rune(8207))
	leftToRightOverride := string(rune(8238))
	leftToRightIsolate := string(rune(8294))
	wordJoiner := string(rune(8288))
	byteOrderMark := string(rune(65279))

	tests := map[string]string{
		"a space":                  "https://evil.example/ login",
		"a tab":                    "https://evil.example/\tlogin",
		"a NUL byte":               "https://evil.example/\x00login",
		"a zero-width space":       "https://evil.example/" + zeroWidthSpace + "login",
		"a right-to-left mark":     "https://evil.example/" + rightToLeftMark + "login",
		"a left-to-right override": "https://evil" + leftToRightOverride + "example.com/login",
		"a left-to-right isolate":  "https://evil" + leftToRightIsolate + "example.com/login",
		"a word joiner":            "https://evil" + wordJoiner + "example.com/login",
		"a byte-order mark":        "https://paypal-login.ru" + byteOrderMark + ".paypal.com/login",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if domain.AllowedLinkURL(url) {
				t.Errorf("AllowedLinkURL(%q) = true, want false", url)
			}
		})
	}
}

// TestAllowedLinkURL_KeepsMailtoAndTgRulesUnchanged confirms mailto:
// and tg: links, which carry no "//" authority of their own, are
// never held to the http(s)-only authority rule - only the scheme
// allow-list and the hidden-character rule, which already applied to
// every scheme, still apply.
func TestAllowedLinkURL_KeepsMailtoAndTgRulesUnchanged(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"mailto with no query string": "mailto:a@example.com",
		"mailto with a query string":  "mailto:a@example.com?body=hello",
		"tg with a query string":      "tg://resolve?domain=x",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !domain.AllowedLinkURL(url) {
				t.Errorf("AllowedLinkURL(%q) = false, want true", url)
			}
		})
	}
}
