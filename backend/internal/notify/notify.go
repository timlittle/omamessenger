// Package notify provides desktop notifications without coupling the app to
// a particular notification daemon implementation.
package notify

import "os/exec"

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
