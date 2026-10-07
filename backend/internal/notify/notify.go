// Package notify shows desktop notifications through notify-send, which
// Omarchy's notification daemon displays.
package notify

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"time"
)

// Desktop shows notifications with notify-send and reports when the user
// clicks one.
type Desktop struct {
	// Command builds the process that shows one notification. Nil uses
	// exec.CommandContext; tests replace it with a stub so nothing real
	// runs.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd

	// Click is called with a notification's conversation id when the
	// user chooses its "default" action: clicking the notification
	// itself, rather than closing or ignoring it. Nil means clicks are
	// not reported.
	Click func(conversationID string)
}

// notifyTimeout bounds how long Notify's goroutine waits for notify-send,
// which otherwise blocks until the notification closes or is clicked. It
// exists only so that goroutine cannot leak forever if a notification is
// somehow never closed; it is not meant to cut a real notification short.
const notifyTimeout = 5 * time.Minute

// Notify shows title and body as a notification carrying conversationID,
// and waits for it in its own goroutine so the caller never blocks.
// notify-send blocks until the notification closes or its "default"
// action is chosen, printing "default" to stdout when it is; Notify
// reports that to Click. A missing notify-send or a refused notification
// is ignored: a notification is never worth failing a message over.
func (d Desktop) Notify(title, body, conversationID string) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	cmd := d.run(ctx, "notify-send",
		"--app-name=OmaMessenger", "--category=im.received", "--action=default=Open",
		"--", title, body)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return
	}

	go d.wait(cancel, cmd, stdout, conversationID)
}

// run builds the notify-send process through Command, or exec.CommandContext
// when none was set.
func (d Desktop) run(ctx context.Context, name string, args ...string) *exec.Cmd {
	if d.Command != nil {
		return d.Command(ctx, name, args...)
	}

	return exec.CommandContext(ctx, name, args...)
}

// wait reads the action notify-send reports on stdout, tells Click about a
// "default" one, then reaps the process so it does not linger as a
// zombie. cancel always runs, releasing notifyTimeout's context as soon as
// notify-send exits.
func (d Desktop) wait(cancel context.CancelFunc, cmd *exec.Cmd, stdout io.Reader, conversationID string) {
	defer cancel()

	clicked := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if scanner.Text() == "default" {
			clicked = true
		}
	}

	_ = cmd.Wait() // notify-send already exited; nothing left to report

	if clicked && d.Click != nil {
		d.Click(conversationID)
	}
}
