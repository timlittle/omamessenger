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

// Media kinds a message can carry besides its text.
const (
	MediaLink  = "link"
	MediaPhoto = "photo"
	MediaVideo = "video"
	MediaFile  = "file"
)

// ErrNotFound reports a missing account, contact, conversation or message.
var ErrNotFound = errors.New("not found")

// Service is a messaging service the helper can add an account for.
type Service struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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
	Pinned        bool   `json:"pinned"`
	Archived      bool   `json:"archived"`
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
	Media          *Media `json:"media,omitempty"`

	// Edited is true once the service reports this message changed after
	// it was first sent.
	Edited bool `json:"edited,omitempty"`

	// Reactions are the emoji chips shown under the message, as the
	// service currently reports them.
	Reactions []Reaction `json:"reactions,omitempty"`
}

// Reaction is one emoji reaction to a message: how many people picked
// it, and whether the signed-in user is one of them.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// Media is what a message carries besides its text: a link preview, a
// photo, a video or a file. Thumb is a small JPEG preview in base64, sent
// with the message; a full photo, video or file is fetched only when the
// user wants it. Duration is in seconds and Size in bytes.
type Media struct {
	Kind        string `json:"kind"`
	URL         string `json:"url,omitempty"`
	SiteName    string `json:"siteName,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Thumb       string `json:"thumb,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Duration    int    `json:"duration,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// ValidService reports whether service is a supported provider.
func ValidService(service string) bool {
	return service == ServiceWhatsApp || service == ServiceTelegram
}
