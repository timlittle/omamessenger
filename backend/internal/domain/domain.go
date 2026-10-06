// Package domain holds the normalized shapes shared by the store, the
// connectors and the UI protocol. Nothing here knows about WhatsApp or
// Telegram wire formats.
package domain

import "errors"

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

// ErrNotFound reports a missing account, contact, conversation or message.
var ErrNotFound = errors.New("not found")

// Account is one signed-in messaging account.
type Account struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

// Contact is a person the account can start a conversation with.
type Contact struct {
	AccountID string `json:"accountId"`
	RemoteID  string `json:"remoteId"`
	Name      string `json:"name"`
}

// Conversation is a direct chat or group, with the summary the
// conversation list shows.
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

// Message is one message in a conversation. Created is in Unix
// milliseconds.
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

// ValidService reports whether service is a supported provider.
func ValidService(service string) bool {
	return service == ServiceWhatsApp || service == ServiceTelegram
}
