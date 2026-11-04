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
// offers: images and video first, then every other file.
var attachFilters = []portal.Filter{
	{Name: "Images and video", Globs: []string{"*.jpg", "*.jpeg", "*.png", "*.gif", "*.webp", "*.heic", "*.mp4", "*.mov", "*.webm", "*.mkv"}},
	{Name: "All files", Globs: []string{"*"}},
}

// soundFilters are the audio formats the notification sound picker
// offers; ffmpeg re-encodes whichever one the user picks into WAV.
var soundFilters = []portal.Filter{
	{Name: "Audio files", Globs: []string{"*.mp3", "*.wav", "*.ogg", "*.oga", "*.m4a", "*.flac"}},
}

// FilePickError reports that no file picker dialog could be shown.
// Reason is the only detail that crosses the protocol.
type FilePickError struct {
	Reason string
	err    error
}

// Error renders the failure for the helper's own logs.
func (e *FilePickError) Error() string {
	return fmt.Sprintf("file picker: %s: %v", e.Reason, e.err)
}

// Unwrap exposes the underlying failure for errors.Is/As.
func (e *FilePickError) Unwrap() error {
	return e.err
}

// errFilePickerNotWired reports that this helper build has no
// FilePicker at all.
var errFilePickerNotWired = errors.New("no file picker wired in")

// PickFile opens a file-choosing dialog for purpose ("attach" or
// "sound", which decides the filters offered) and returns the chosen
// path, or "" with no error if the user cancelled. It fails with a
// *FilePickError when no FilePicker is wired in or the dialog could
// not be shown.
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
