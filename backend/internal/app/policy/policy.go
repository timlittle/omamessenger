// Package policy holds the pure decisions made when a live message arrives:
// whether to mark it read, whether to notify, and what the notification says.
// It performs no I/O so every combination can be tested exhaustively.
package policy

import "github.com/timlittle/omamessenger/backend/internal/domain"

// Input describes one arriving message and the state it arrives into.
type Input struct {
	Notifications bool // the user's notification setting
	Preview       bool // show message text in notifications
	Muted         bool // the conversation is muted
	Focused       bool // the conversation is the one open in the window
	WindowActive  bool // the window has focus
	Kind          string
	Sender        string
	Title         string
	Text          string
}

// MarkReadOnArrival reports whether the user is looking at the conversation,
// so the message is read the moment it arrives.
func MarkReadOnArrival(in Input) bool {
	return in.WindowActive && in.Focused
}

// ShouldNotify reports whether to raise a desktop notification.
func ShouldNotify(in Input) bool {
	return in.Notifications && !in.Muted && !MarkReadOnArrival(in)
}

// Notification returns the title and body for a desktop notification. Groups
// name the conversation; the body hides the text unless previews are on.
func Notification(in Input) (title, body string) {
	title = in.Sender
	if in.Kind == domain.KindGroup {
		title += " · " + in.Title
	}
	body = "New message"
	if in.Preview {
		body = in.Text
	}
	return title, body
}
