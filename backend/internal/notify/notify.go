// Package notify shows desktop notifications through notify-send, which
// Omarchy's notification daemon displays.
package notify

import "os/exec"

// Desktop shows notifications with notify-send.
type Desktop struct{}

// Notify shows a notification without waiting for it. A missing notify-send
// or a refused notification is ignored: a notification is never worth
// failing a message over.
func (Desktop) Notify(title, body string) {
	cmd := exec.Command("notify-send", "--app-name=OmaMessenger", "--category=im.received", "--", title, body)
	if err := cmd.Start(); err != nil {
		return
	}

	// Reap the process so it does not linger as a zombie.
	go func() { _ = cmd.Wait() }()
}
