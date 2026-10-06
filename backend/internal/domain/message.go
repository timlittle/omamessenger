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
