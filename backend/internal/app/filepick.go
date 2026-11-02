package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/portal"
)

// filePickTitle is used when a files.pick request gives no title of
// its own.
const filePickTitle = "Choose a file"

// attachFilters are the file kinds the composer's attach button
// offers: images and video first, matching what a message can
// actually carry, then every other file.
var attachFilters = []portal.Filter{
	{Name: "Images and video", Globs: []string{"*.jpg", "*.jpeg", "*.png", "*.gif", "*.webp", "*.heic", "*.mp4", "*.mov", "*.webm", "*.mkv"}},
	{Name: "All files", Globs: []string{"*"}},
}

// soundFilters are the audio formats the notification sound picker
// offers; audiosafe itself is the real authority on what actually
// plays (see notificationsound.go), this only steers the dialog
// towards the formats that stand a chance. MP3 is deliberately left
// out: audiosafe never reports an MP3 playable (see its own package
// doc comment and docs/decisions.md), so offering it here would only
// invite a pick errSoundNotPlayable always refuses.
var soundFilters = []portal.Filter{
	{Name: "Audio files", Globs: []string{"*.ogg", "*.oga", "*.m4a"}},
}

// FilePickError reports that no file picker dialog could be shown at
// all, the UI's own cue (see server/errors.go) to fall back to a
// different file picker - never to the in-process GTK dialog that
// once crashed the whole Omarchy shell (see docs/decisions.md).
// Reason is the only detail that crosses the protocol: a safe, fixed
// category, never the underlying dbus failure's own text.
type FilePickError struct {
	Reason string
	err    error
}

// Error renders the failure for the helper's own logs; only Reason
// ever reaches the UI (see server/errors.go).
func (e *FilePickError) Error() string {
	return fmt.Sprintf("file picker: %s: %v", e.Reason, e.err)
}

// Unwrap exposes the underlying failure for errors.Is/As, without it
// ever crossing the protocol itself.
func (e *FilePickError) Unwrap() error {
	return e.err
}

// errFilePickerNotWired reports that this helper build has no
// FilePicker at all; wrapped in a *FilePickError, reason "unavailable",
// the same as any other failure to show the dialog.
var errFilePickerNotWired = errors.New("no file picker wired in")

// PickFile opens a file-choosing dialog for purpose ("attach" or
// "sound", which decides the filters offered) and returns the local
// path the user chose, or "" with no error if they cancelled: a
// cancelled pick is never a failure. Fails with a *FilePickError,
// reason "unavailable", when no FilePicker was wired in or the dialog
// itself could not be shown, which is the UI's own cue to fall back to
// a different file picker rather than the in-process GTK dialog that
// once crashed the whole shell (see docs/decisions.md).
func (c *Commands) PickFile(ctx context.Context, purpose, title string) (string, error) {
	if c.filePicker == nil {
		return "", &FilePickError{Reason: "unavailable", err: errFilePickerNotWired}
	}

	if title == "" {
		title = filePickTitle
	}

	path, ok, err := c.filePicker.Pick(ctx, title, filtersForPurpose(purpose))
	if err != nil {
		return "", &FilePickError{Reason: "unavailable", err: err}
	}
	if !ok {
		return "", nil
	}

	return path, nil
}

// filtersForPurpose maps a files.pick purpose to the filters its
// dialog offers; an unknown purpose offers none, letting every file
// through.
func filtersForPurpose(purpose string) []portal.Filter {
	switch purpose {
	case "sound":
		return soundFilters
	case "attach":
		return attachFilters
	default:
		return nil
	}
}
