// Package notify provides desktop notifications without coupling the app to
// a particular notification daemon implementation.
package notify

import (
	"os/exec"
	"sync"
)

// Notifier sends a desktop notification.
type Notifier interface {
	Notify(title, body string)
}

// Desktop delegates notification display to notify-send. A missing daemon
// utility or a rejected notification is intentionally non-fatal.
type Desktop struct{}

func (Desktop) Notify(title, body string) {
	cmd := exec.Command("notify-send", "--app-name=OmaMessenger", "--category=im.received", "--", title, body)
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { _ = cmd.Wait() }()
}

// Notification is one recorded call to Recorder.Notify.
type Notification struct {
	Title string
	Body  string
}

// Recorder is a concurrency-safe test notifier.
type Recorder struct {
	mu            sync.Mutex
	notifications []Notification
}

func (r *Recorder) Notify(title, body string) {
	r.mu.Lock()
	r.notifications = append(r.notifications, Notification{Title: title, Body: body})
	r.mu.Unlock()
}

// Calls returns a snapshot of recorded notifications.
func (r *Recorder) Calls() []Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Notification(nil), r.notifications...)
}
