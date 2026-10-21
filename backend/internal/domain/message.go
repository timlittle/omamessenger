package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Message delivery states. Outgoing messages move forward through pending,
// sent, delivered and read; every incoming message is received.
const (
	StatusPending   = "pending"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
	StatusReceived  = "received"
)

// MaxTextLength bounds outgoing text in characters. Both services accept at
// least this much.
const MaxTextLength = 4096

// MaxIncomingTextLength bounds a received message's text in bytes, far
// more generous than MaxTextLength since it exists only to cap a
// sender-controlled field, not to match what a real client would ever
// send: a message with no length limit at all could otherwise make the
// UI lay out an unbounded amount of rich text in one bubble (see
// ui/lib/Markdown.js and docs/decisions.md).
const MaxIncomingTextLength = 64 * 1024

// incomingTruncatedSuffix is appended, within the same cap, when
// TruncateIncomingText actually had to cut a message's text, so the
// user sees why it ends abruptly rather than assuming the sender's own
// message really stopped there.
const incomingTruncatedSuffix = "\n\n[message truncated]"

// TruncateIncomingText cuts text to MaxIncomingTextLength bytes, never
// splitting a UTF-8 rune, and appends incomingTruncatedSuffix within
// that same cap when it actually had to cut anything. Unlike
// MaxTextLength, which rejects outgoing text outright, a received
// message is never dropped for being too long, only shortened: the
// sender is not asking permission the way the local user is when
// sending.
func TruncateIncomingText(text string) string {
	if len(text) <= MaxIncomingTextLength {
		return text
	}

	limit := MaxIncomingTextLength - len(incomingTruncatedSuffix)
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}

	return text[:limit] + incomingTruncatedSuffix
}

// Errors reported when outgoing text fails validation; see
// app.captionText.
var (
	ErrEmptyText   = errors.New("message text is empty")
	ErrTextTooLong = errors.New("message text is too long")
)

// statusRank orders the forward delivery states.
var statusRank = map[string]int{
	StatusPending:   1,
	StatusSent:      2,
	StatusDelivered: 3,
	StatusRead:      4,
}

// StatusAdvances reports whether moving from one delivery state to another
// is forward progress. Receipts can arrive out of order, so a late
// "delivered" must not overwrite "read". A failed message may be retried.
func StatusAdvances(from, to string) bool {
	if from == to {
		return false
	}

	if to == StatusFailed {
		return from == StatusPending || from == StatusSent
	}

	if from == StatusFailed {
		return to == StatusPending || to == StatusSent
	}

	f, okFrom := statusRank[from]
	t, okTo := statusRank[to]

	return okFrom && okTo && t > f
}

// ExcerptLength bounds the quote Excerpt keeps, in characters.
const ExcerptLength = 80

// Excerpt is the short, single-line quote shown above a reply: the first
// line of text, trimmed, cut to ExcerptLength characters with a trailing
// "…" if it was longer.
func Excerpt(text string) string {
	if i := strings.IndexAny(text, "\r\n"); i != -1 {
		text = text[:i]
	}

	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= ExcerptLength {
		return text
	}

	return string(runes[:ExcerptLength]) + "…"
}

// MediaPlaceholder is the caption a photo, video or file gets when the
// user sends it without one, since every stored message has text. The UI
// hides a caption that exactly matches its media's placeholder (see
// Format.caption in ui/lib), and a connector never sends the placeholder
// itself as the real caption (see the Telegram connector's Send).
func MediaPlaceholder(kind string) string {
	switch kind {
	case MediaPhoto:
		return "[Photo]"
	case MediaVideo:
		return "[Video]"
	case MediaVoice:
		return "[Voice message]"
	case MediaSticker:
		return "[Sticker]"
	default:
		return "[File]"
	}
}
