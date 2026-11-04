// Package portal opens a file-choosing dialog through
// org.freedesktop.portal.FileChooser on the session bus, so the dialog
// runs in the portal's own process rather than crashing the shared
// Omarchy shell (docs/decisions.md).
package portal

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/godbus/dbus/v5"
)

// ErrCancelled reports the user closed the dialog without picking a
// file.
var ErrCancelled = errors.New("portal: cancelled")

// pickTimeout bounds how long Pick waits for the user to answer the
// dialog, so a forgotten dialog can never leak the request, or the
// goroutine the server's own dispatch runs it on, forever.
const pickTimeout = 10 * time.Minute

// Filter is one named group of file extensions a dialog offers, such
// as {"Images and video", []string{"*.jpg", "*.mp4"}}.
type Filter struct {
	Name  string
	Globs []string
}

// Bus is the seam between Picker and a real D-Bus session bus
// connection. dialSessionBus is the production implementation; tests
// use a hand-written fake so no test opens a real dialog.
type Bus interface {
	// OpenFile calls FileChooser.OpenFile and returns the request
	// object path its Response signal will carry.
	OpenFile(ctx context.Context, title string, filters []Filter) (dbus.ObjectPath, error)

	// Wait blocks until a Response signal for path arrives, returning
	// the chosen file's own file:// URI, ErrCancelled if the user
	// cancelled, or ctx's error if it ends first.
	Wait(ctx context.Context, path dbus.ObjectPath) (string, error)
}

// Picker opens the portal's own FileChooser dialog.
type Picker struct {
	// Dial opens the Bus Pick talks to. Nil uses dialSessionBus; tests
	// replace it with a fake so nothing reaches a real session bus.
	Dial func(ctx context.Context) (Bus, func(), error)
}

// Pick opens title over the portal's FileChooser, restricted to
// filters, and returns the chosen file's local path. ok is false with
// no error when the user cancelled; err is returned only when the
// dialog itself could not be shown, such as no portal being reachable.
func (p *Picker) Pick(ctx context.Context, title string, filters []Filter) (path string, ok bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, pickTimeout)
	defer cancel()

	dial := p.Dial
	if dial == nil {
		dial = dialSessionBus
	}

	bus, release, err := dial(ctx)
	if err != nil {
		return "", false, fmt.Errorf("portal: dial session bus: %w", err)
	}
	defer release()

	reqPath, err := bus.OpenFile(ctx, title, filters)
	if err != nil {
		return "", false, fmt.Errorf("portal: open file chooser: %w", err)
	}

	uri, err := bus.Wait(ctx, reqPath)
	if errors.Is(err, ErrCancelled) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	path, err = decodeFileURI(uri)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}

// decodeFileURI converts a file:// URI the portal returned into a
// local path, refusing any other scheme.
func decodeFileURI(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("portal: parse uri: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("portal: refusing a non-file uri")
	}
	if u.Path == "" {
		return "", fmt.Errorf("portal: file uri names no path")
	}

	return u.Path, nil
}
