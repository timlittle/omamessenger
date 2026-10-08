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

// Media kinds a message can carry besides its text. MediaVoice is a voice
// note (WhatsApp's push-to-talk audio, Telegram's voice message), kept
// apart from MediaFile so the UI can offer an inline player instead of a
// plain "open externally" file row. MediaSticker is a static image shown
// borderless and unbubbled; an animated sticker (WhatsApp's animated
// WebP, Telegram's Lottie or video stickers) still normalizes to this
// kind, but a connector that cannot show one as a still image leaves
// FileName empty so the UI never tries to fetch and decode it, falling
// back to Thumb or Emoji instead (see Media's own doc comment).
const (
	MediaLink    = "link"
	MediaPhoto   = "photo"
	MediaVideo   = "video"
	MediaFile    = "file"
	MediaVoice   = "voice"
	MediaSticker = "sticker"
)

// ErrNotFound reports a missing account, contact, conversation or message.
var ErrNotFound = errors.New("not found")

// ErrMediaDecryptFailed reports a downloaded attachment that failed to
// verify or decrypt: the service answered, but the bytes it sent no
// longer match the key or hash the message arrived with, for example
// because the reference has expired. A connector wraps its own
// library's decrypt error with this sentinel so the app layer can tell
// the category apart from an outright download failure.
var ErrMediaDecryptFailed = errors.New("media decrypt failed")

// ErrMediaExpired reports that a message's media can no longer be
// fetched at all: the service's own copy has aged out and the device
// that could still have one, such as WhatsApp's primary phone, either
// confirmed it is gone or never answered a request to check. A
// connector wraps this sentinel once it has tried everything it can; the
// app layer reports it as its own reason category, distinct from a
// plain download failure that might still succeed on a later attempt.
var ErrMediaExpired = errors.New("media no longer available")

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
	Hidden        bool   `json:"hidden"`
	LastActivity  int64  `json:"lastActivity"`

	// ReminderAt is when a snoozed conversation comes back, in Unix
	// milliseconds, or 0 for none. It is local to this computer only:
	// never reported to or read from the service.
	ReminderAt int64 `json:"reminderAt"`

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

	// ReplyTo is the message this one answers, or nil when it answers
	// nothing.
	ReplyTo *Reply `json:"replyTo,omitempty"`

	// Mentions are the "@name" tokens in Text, each naming a group
	// member. An outgoing message carries whatever the composer sent;
	// an incoming one carries whatever the connector found in the
	// service's own mention metadata.
	Mentions []Mention `json:"mentions,omitempty"`

	// MentionsMe is true when this message's Mentions name the
	// signed-in account itself, precomputed by the connector, which
	// already knows its own identity, the same way Reaction.Mine is.
	MentionsMe bool `json:"mentionsMe,omitempty"`
}

// Reaction is one emoji reaction to a message: how many people picked
// it, and whether the signed-in user is one of them.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// Reply is the message a reply answers: enough to show a quote above the
// reply's own text, and, for an outgoing reply, enough for a connector to
// thread it under the original on the service. SenderName and Text are
// filled in from the quoted message once it is known locally; a
// connector reporting an incoming reply may have only RemoteID.
type Reply struct {
	RemoteID   string `json:"remoteId"`
	SenderName string `json:"senderName,omitempty"`
	Text       string `json:"text,omitempty"`
}

// Media is what a message carries besides its text: a link preview, a
// photo, a video, a file, a voice note or a sticker. Thumb is a small
// preview image in base64 (JPEG for every kind but a sticker, which
// carries whatever format its service's own thumbnail used), sent with
// the message; a full photo, video, file or voice note is fetched only
// when the user wants it. Duration is in seconds and Size in bytes; a
// voice note always carries Duration, for its player's elapsed time
// label before playback has started. Emoji is a sticker's associated
// emoji, shown when neither Thumb nor a fetchable FileName can: a
// Lottie or video sticker this UI will never try to decode.
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
	Emoji       string `json:"emoji,omitempty"`

	// Path is where an outgoing attachment's file sits in the helper's
	// own outgoing media area, for a connector to read and upload. It is
	// never the user's original path, which they might move, rename or
	// delete after picking it, and it never crosses the protocol: the UI
	// already knows the path it offered, and a remote one holds nothing
	// meaningful on this machine, so json:"-" keeps it local rather than
	// adding a field every other message would carry as empty.
	Path string `json:"-"`
}

// ValidService reports whether service is a supported provider.
func ValidService(service string) bool {
	return service == ServiceWhatsApp || service == ServiceTelegram
}
