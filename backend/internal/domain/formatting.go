package domain

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
