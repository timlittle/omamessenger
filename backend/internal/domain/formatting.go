package domain

import (
	"strings"
	"unicode"
)

// LinkSchemes are the URL schemes a link - a formatting span, a link
// preview, or anything else a connector reports as clickable - may
// open. A sender fully controls the URL behind a link span and, on
// WhatsApp, a link preview's own URL (see AllowedLinkURL), so this is
// the one place every connector checks before ever reporting a URL the
// UI will open with no filter of its own.
var LinkSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tg": true}

// AllowedLinkURL reports whether url is safe to open: its scheme is
// one LinkSchemes allows, it carries none of hasHiddenOrUnusualRune's
// own characters, and, for http(s), has a real "scheme://authority"
// shape (see hasHTTPAuthority) rather than relying on a browser's own
// forgiving parser to find a host some other way. A connector drops a
// link entirely, never reporting a URL whose scheme, characters or
// shape this rejects, rather than reporting it unfiltered and relying
// on the UI to check it again; the UI's own ui/lib/Format.js spanURL
// mirrors this exact rule, so a link can never be shown as safe on one
// side and opened on the other (see docs/decisions.md).
func AllowedLinkURL(url string) bool {
	scheme, rest, ok := strings.Cut(url, ":")
	if !ok {
		return false
	}

	scheme = strings.ToLower(scheme)
	if !LinkSchemes[scheme] {
		return false
	}

	if strings.ContainsFunc(url, hasHiddenOrUnusualRune) {
		return false
	}

	isHTTP := scheme == "http" || scheme == "https"

	return !isHTTP || hasHTTPAuthority(rest)
}

// hasHiddenOrUnusualRune reports whether r is a backslash, a Unicode
// space or control character, or one of the Unicode "format"
// characters isHiddenFormatRune lists: none of these ever belongs in
// a real link, so AllowedLinkURL refuses a URL carrying any of them
// outright, rather than opening it on the strength of a confirmation
// that cannot show what such a character actually hides (see
// docs/decisions.md). A backslash matters on its own because a
// browser's own WHATWG URL parser ends an http(s) authority at one
// the same as it does a forward slash, while this package's and
// ui/lib/Links.js's own host-parsing never did.
func hasHiddenOrUnusualRune(r rune) bool {
	return r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) || isHiddenFormatRune(r)
}

// hiddenFormatRuneRanges are the inclusive [low, high] Unicode code
// point ranges isHiddenFormatRune checks: the specific "format"
// characters (category Cf) a spoofed link has used to hide or split
// its own destination - zero-width spaces and marks, bidi embeddings
// and overrides, word joiner and invisible operators, bidi isolates,
// and the byte-order mark.
var hiddenFormatRuneRanges = [][2]rune{
	{0x200B, 0x200F},
	{0x202A, 0x202E},
	{0x2060, 0x2064},
	{0x2066, 0x2069},
	{0xFEFF, 0xFEFF},
}

// isHiddenFormatRune reports whether r falls in one of
// hiddenFormatRuneRanges.
func isHiddenFormatRune(r rune) bool {
	for _, rg := range hiddenFormatRuneRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}

	return false
}

// hasHTTPAuthority reports whether rest - url with its scheme and the
// colon after it already cut off - starts with "//" followed by at
// least one character that is not itself "/", "?" or "#": the one
// shape an allowed http(s) URL's own authority must take. A browser's
// own URL parser still resolves a host for "http:evil.example",
// "https:\\evil.example" or "https:/\evil.example" - each missing a
// real "//" authority of its own, or carrying a backslash standing in
// for part of it - so refusing every one of them here, rather than
// relying only on hasHiddenOrUnusualRune's backslash check, is what
// keeps this package's own idea of a URL's shape from ever disagreeing
// with the browser that will actually open it.
func hasHTTPAuthority(rest string) bool {
	authority, ok := strings.CutPrefix(rest, "//")
	if !ok {
		return false
	}

	return authority != "" && authority[0] != '/' && authority[0] != '?' && authority[0] != '#'
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
