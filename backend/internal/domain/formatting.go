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

// AllowedLinkURL reports whether url is safe to open: its scheme is one
// LinkSchemes allows, it has no hidden or unusual runes, and, for
// http(s), a real "scheme://authority" shape (see hasHTTPAuthority).
// ui/lib/Format.js's spanURL mirrors this rule (docs/decisions.md).
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
// space or control character, or one of isHiddenFormatRune's "format"
// characters: none of these ever belongs in a real link. A backslash
// matters because a browser's WHATWG URL parser ends an http(s)
// authority at one, the same as a forward slash.
func hasHiddenOrUnusualRune(r rune) bool {
	return r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) || isHiddenFormatRune(r)
}

// hiddenFormatRuneRanges are the inclusive [low, high] Unicode "format"
// (category Cf) ranges a spoofed link could use to hide or split its
// destination: zero-width spaces and marks, bidi controls, word joiner,
// invisible operators and the byte-order mark.
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

// hasHTTPAuthority reports whether rest (url with its scheme and colon
// already cut off) starts with "//" followed by a character that is
// not "/", "?" or "#": the shape an allowed http(s) authority must
// take, rejecting forms like "http:evil.example" a browser would still
// resolve a host for.
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
