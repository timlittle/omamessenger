package fake

import (
	"fmt"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// seedWindow is how far back seeded history reaches.
const seedWindow = 6 * 24 * time.Hour

// conversation returns the normalized conversation for the script.
func (s conversationScript) conversation(accountID string) domain.Conversation {
	return domain.Conversation{
		AccountID: accountID, RemoteID: s.remoteID, Kind: s.kind,
		Title: s.title, Members: s.members, Muted: s.muted,
	}
}

// history returns the seeded messages, spread evenly over the seed window
// before now, oldest first.
func (s conversationScript) history(now time.Time) []domain.Message {
	step := seedWindow / time.Duration(s.count+1)
	messages := make([]domain.Message, s.count)

	for i := range messages {
		outgoing := s.isOutgoing(i)
		senderID, senderName := s.sender(i, outgoing)
		messages[i] = domain.Message{
			RemoteID:   fmt.Sprintf("seed-%s-%d", s.remoteID, i+1),
			SenderID:   senderID,
			SenderName: senderName,
			Text:       s.texts[i%len(s.texts)],
			Outgoing:   outgoing,
			Status:     s.status(i, outgoing),
			Created:    now.Add(-seedWindow + time.Duration(i+1)*step).UnixMilli(),
		}
	}

	s.applyScriptedMedia(messages)

	return messages
}

// applyScriptedMedia turns the newest message into a photo or a voice
// note when the script asks for one, and attaches a link preview to
// whichever seeded text names it, mutating messages in place. Split out
// of history to keep that function's own branching within the house
// style's complexity limit.
func (s conversationScript) applyScriptedMedia(messages []domain.Message) {
	s.applyNewestMedia(messages)

	if s.linkPreviewText != "" {
		for i := range messages {
			if messages[i].Text == s.linkPreviewText {
				messages[i].Media = linkPreviewMedia()
			}
		}
	}
}

// applyNewestMedia turns the newest message into a photo, a voice note
// or a real GIF document when the script asks for one - at most one of
// the three, since each overwrites the same message's text and media.
// Split out of applyScriptedMedia to keep that function's own branching
// within the house style's complexity limit.
func (s conversationScript) applyNewestMedia(messages []domain.Message) {
	if s.count == 0 {
		return
	}

	newest := &messages[s.count-1]
	switch {
	case s.lastPhoto:
		newest.Text = "[Photo]"
		newest.Media = &domain.Media{Kind: domain.MediaPhoto, Width: photoWidth, Height: photoHeight}

		if older := s.count - 1 - s.extraPhotoOffset; s.extraPhotoOffset > 0 && older >= 0 {
			messages[older].Text = "[Photo]"
			messages[older].Media = &domain.Media{Kind: domain.MediaPhoto, Width: 300, Height: 400}
		}
	case s.lastVoice:
		newest.Text = "[Voice message]"
		newest.Media = &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg", Duration: 5}
	case s.lastGif:
		newest.Text = "[GIF]"
		// Kind and Animated match exactly what a real Telegram or
		// WhatsApp GIF document normalizes to (see
		// telegram/media.go's and whatsapp/normalize_media.go's own
		// isGIFImage case): a classification hint only, never what
		// the UI trusts to decode it - FetchMedia below writes a
		// genuine GIF file, which app.DecodeAsForMedia confirms the
		// same way it would a real sender's.
		newest.Media = &domain.Media{Kind: domain.MediaVideo, FileName: "run.gif", Animated: true}
	}
}

// linkPreviewMedia is the stand-in link preview a real service would have
// fetched from the page a seeded message links to.
func linkPreviewMedia() *domain.Media {
	return &domain.Media{
		Kind: domain.MediaLink, URL: "https://example.com/tickets",
		SiteName: "example.com", Title: "Event tickets",
		Description: "Two tickets, front row, Saturday night.",
	}
}

// isUnread reports whether message i is one of the trailing unread ones.
func (s conversationScript) isUnread(i int) bool {
	return i >= s.count-s.unread
}

// isOutgoing alternates incoming and outgoing messages. Unread messages are
// always incoming.
func (s conversationScript) isOutgoing(i int) bool {
	if s.isUnread(i) {
		return false
	}

	return s.allOutgoing || i%2 == 1 || (s.lastOutgoing && i == s.count-1)
}

// sender returns who sent message i: the user, a group member in turn, or
// the other person in a direct chat.
func (s conversationScript) sender(i int, outgoing bool) (id, name string) {
	switch {
	case outgoing:
		return "self", "You"
	case len(s.groupSenders) > 0:
		name := s.groupSenders[i%len(s.groupSenders)]
		return name, name
	default:
		return s.remoteID, s.title
	}
}

// status is delivered for group sends, read for direct sends and older
// incoming messages, and received for unread ones.
func (s conversationScript) status(i int, outgoing bool) string {
	switch {
	case outgoing && s.kind == domain.KindGroup:
		return domain.StatusDelivered
	case outgoing, !s.isUnread(i):
		return domain.StatusRead
	default:
		return domain.StatusReceived
	}
}

// olderHistory returns the older messages, an hour apart before the seed
// window, oldest first.
func (s conversationScript) olderHistory(now time.Time) []domain.Message {
	start := now.Add(-seedWindow)
	messages := make([]domain.Message, s.older)

	for i := range messages {
		messages[i] = domain.Message{
			RemoteID:   fmt.Sprintf("older-%s-%d", s.remoteID, i+1),
			SenderID:   s.title,
			SenderName: s.title,
			Text:       s.texts[i%len(s.texts)],
			Status:     domain.StatusReceived,
			Created:    start.Add(-time.Duration(s.older-i) * time.Hour).UnixMilli(),
		}
	}

	return messages
}

// isVoiceNote reports whether messageRemoteID names the voice note
// applyScriptedMedia seeds as the newest message of the conversation
// convRemoteID: the fake's own record of a message's kind, so FetchMedia
// never has to guess it from the cache's file name.
func (a accountScript) isVoiceNote(convRemoteID, messageRemoteID string) bool {
	for _, s := range a.conversations {
		if s.remoteID == convRemoteID && s.lastVoice && s.count > 0 {
			return messageRemoteID == fmt.Sprintf("seed-%s-%d", s.remoteID, s.count)
		}
	}

	return false
}

// isGifDocument reports whether messageRemoteID names the real GIF
// document applyScriptedMedia seeds as the newest message of the
// conversation convRemoteID, the same way isVoiceNote tells FetchMedia
// which placeholder bytes a scripted message needs.
func (a accountScript) isGifDocument(convRemoteID, messageRemoteID string) bool {
	for _, s := range a.conversations {
		if s.remoteID == convRemoteID && s.lastGif && s.count > 0 {
			return messageRemoteID == fmt.Sprintf("seed-%s-%d", s.remoteID, s.count)
		}
	}

	return false
}
