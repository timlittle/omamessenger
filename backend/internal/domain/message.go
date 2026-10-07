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

// Errors returned by NormalizeOutgoingText.
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

// NormalizeOutgoingText trims surrounding whitespace and checks the length.
func NormalizeOutgoingText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmptyText
	}

	if utf8.RuneCountInString(text) > MaxTextLength {
		return "", ErrTextTooLong
	}

	return text, nil
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
	default:
		return "[File]"
	}
}
