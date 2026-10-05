// Package domain holds the normalized shapes shared by the store, the
// connectors, and the UI protocol. Nothing here knows about WhatsApp or
// Telegram wire formats.
package domain

import (
	"errors"
	"strings"
)

// Services the UI knows how to present.
const (
	ServiceWhatsApp = "whatsapp"
	ServiceTelegram = "telegram"
)

// Account connection states.
const (
	AccountConnecting = "connecting"
	AccountConnected  = "connected"
	AccountNeedsAuth  = "needs-auth"
	AccountError      = "error"
	AccountOffline    = "offline"
)

// Conversation kinds.
const (
	KindDirect = "direct"
	KindGroup  = "group"
)

// Outgoing message delivery states, in the order they normally progress.
const (
	StatusPending   = "pending"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
	// StatusReceived marks every incoming message.
	StatusReceived = "received"
)

var statusRank = map[string]int{
	StatusPending:   1,
	StatusSent:      2,
	StatusDelivered: 3,
	StatusRead:      4,
}

// StatusAdvances reports whether moving from one delivery state to another is
// forward progress. Receipts can arrive out of order; a late "delivered" must
// not overwrite "read". A failed message may be retried back to pending.
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

// ValidService reports whether service is a supported provider.
func ValidService(service string) bool {
	return service == ServiceWhatsApp || service == ServiceTelegram
}

type Account struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

type Contact struct {
	AccountID string `json:"accountId"`
	RemoteID  string `json:"remoteId"`
	Name      string `json:"name"`
}

type Conversation struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	Service       string `json:"service"`
	RemoteID      string `json:"remoteId"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Members       int    `json:"members"`
	Preview       string `json:"preview"`
	PreviewSender string `json:"previewSender"`
	PreviewOut    bool   `json:"previewOutgoing"`
	Unread        int    `json:"unread"`
	Muted         bool   `json:"muted"`
	LastActivity  int64  `json:"lastActivity"`
	// Match is a snippet of the newest message that matched a search query.
	Match string `json:"match,omitempty"`
}

type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	RemoteID       string `json:"remoteId"`
	SenderID       string `json:"senderId"`
	SenderName     string `json:"senderName"`
	Text           string `json:"text"`
	Outgoing       bool   `json:"outgoing"`
	Status         string `json:"status"`
	Created        int64  `json:"created"`
}

// MaxMessageLength bounds outgoing text. Both services accept at least this.
const MaxMessageLength = 4096

var ErrEmptyText = errors.New("message text is empty")
var ErrTextTooLong = errors.New("message text is too long")

// NormalizeOutgoingText trims surrounding whitespace and validates length.
func NormalizeOutgoingText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmptyText
	}
	if len([]rune(text)) > MaxMessageLength {
		return "", ErrTextTooLong
	}
	return text, nil
}
