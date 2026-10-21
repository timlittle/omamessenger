package domain

import "strings"

// LinkSchemes are the URL schemes a link - a formatting span, a link
// preview, or anything else a connector reports as clickable - may
// open. A sender fully controls the URL behind a link span and, on
// WhatsApp, a link preview's own URL (see AllowedLinkURL), so this is
// the one place every connector checks before ever reporting a URL the
// UI will open with no filter of its own.
var LinkSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tg": true}

// AllowedLinkURL reports whether url's scheme is one LinkSchemes
// allows. A connector drops a link entirely, never reporting a URL
// whose scheme this rejects, rather than reporting it unfiltered and
// relying on the UI to check it again.
func AllowedLinkURL(url string) bool {
	scheme, _, ok := strings.Cut(url, ":")

	return ok && LinkSchemes[strings.ToLower(scheme)]
}

// Span is one formatting range over a message's Text: a clickable link, or
// a run of bold, italic, underline, strike, code, pre or spoiler styling.
// Start and End are in UTF-16 code units, the same convention Mention's
// Offset and Length use, so the UI can place a span with no conversion.
// URL is set only when Kind is SpanLink.
type Span struct {
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	URL   string `json:"url,omitempty"`
}

// Span kinds a connector can report and the UI knows how to render.
const (
	SpanLink      = "link"
	SpanBold      = "bold"
	SpanItalic    = "italic"
	SpanUnderline = "underline"
	SpanStrike    = "strike"
	SpanCode      = "code"
	SpanPre       = "pre"
	SpanSpoiler   = "spoiler"
)
