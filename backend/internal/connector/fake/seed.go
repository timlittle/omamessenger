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

	return messages
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
