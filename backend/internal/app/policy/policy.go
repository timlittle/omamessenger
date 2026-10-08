// Package policy holds the pure decisions made when a live message arrives:
// whether to mark it read, whether to notify, and what the notification says.
// It performs no I/O so every combination can be tested exhaustively.
package policy

import "github.com/timlittle/omamessenger/backend/internal/domain"

// Detail is how much a notification reveals about an arriving message.
type Detail string

// The three detail levels a user can choose, from most to least revealing.
const (
	DetailNameAndMessage Detail = "nameAndMessage" // who it is from and the message text
	DetailNameOnly       Detail = "nameOnly"       // who it is from, text hidden
	DetailNone           Detail = "none"           // no name or text, a generic notice
)

// Input describes one arriving message and the state it arrives into.
type Input struct {
	Notifications  bool   // the user's notification setting
	Detail         Detail // how much the notification reveals
	Muted          bool   // the conversation is muted
	Focused        bool   // the conversation is the one open in the window
	WindowActive   bool   // the window has focus
	Missed         bool   // sent well before it arrived, caught up after being offline
	Kind           string
	Sender         string
	Title          string
	Text           string
	ConversationID string // carried in the notification, so a click can reopen it
}

// MarkReadOnArrival reports whether the user is looking at the conversation,
// so the message is read the moment it arrives.
func MarkReadOnArrival(in Input) bool {
	return in.WindowActive && in.Focused
}

// ShouldNotify reports whether to raise a desktop notification. A missed
// message still counts as unread, but notifying for each one would replay
// everything the services kept while the helper was not running.
func ShouldNotify(in Input) bool {
	return in.Notifications && !in.Muted && !in.Missed && !MarkReadOnArrival(in)
}

// Notification returns the title, body and conversation id for a desktop
// notification, shaped by in.Detail. DetailNone hides who it is from too,
// so the title is a generic app name rather than the sender's; the other
// two levels name the sender, and a group also names the conversation,
// with the body showing the message text only at DetailNameAndMessage.
// The conversation id passes straight through, so a click on the
// notification can reopen it.
func Notification(in Input) (title, body, conversationID string) {
	if in.Detail == DetailNone {
		return "OmaMessenger", "New message", in.ConversationID
	}

	title = in.Sender
	if in.Kind == domain.KindGroup {
		title += " · " + in.Title
	}

	body = "New message"
	if in.Detail == DetailNameAndMessage {
		body = in.Text
	}

	return title, body, in.ConversationID
}

// ReminderNotification returns the title, body and conversation id for a
// snoozed conversation's reminder, once it comes due. DetailNone hides
// which chat it is, the same as an arriving message does; the other two
// levels name it, since a reminder carries no message text to hide.
func ReminderNotification(detail Detail, title, conversationID string) (notifTitle, body, convID string) {
	if detail == DetailNone {
		return "OmaMessenger", "Reminder", conversationID
	}

	return "Reminder: " + title, "Snoozed conversation", conversationID
}
