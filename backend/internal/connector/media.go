package connector

import (
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// ErrUnsafeImage reports that a photo, sticker or GIF a connector
// downloaded failed its safety check (see imagesafe) and so was never
// written to disk at all. The app layer reports this the same way as
// any other failed fetch: the UI falls back to a thumbnail or a
// placeholder, never a crash.
var ErrUnsafeImage = errors.New("connector: image failed safety check")

// SaveImageMedia strips compressed or oversized metadata from data,
// which must be a photo, sticker or GIF a connector is about to let
// the UI load into a QML Image or AnimatedImage, and writes the
// result to path, readable only by the user. A sender-controlled
// image is the one kind of downloaded media Qt itself decodes inline,
// which is why every connector's FetchMedia routes exactly these
// kinds through here rather than SaveMedia (see docs/decisions.md).
func SaveImageMedia(path string, data []byte, profile imagesafe.Profile) error {
	safe, ok := imagesafe.Sanitize(data, profile)
	if !ok {
		return fmt.Errorf("connector: save image media: %w", ErrUnsafeImage)
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil {
		return fmt.Errorf("connector: save image media: %w", err)
	}

	return nil
}

// SanitizeOutgoingImage strips compressed or oversized metadata from
// data, an image the user is attaching to an outgoing message - a
// paste from the clipboard, say - and returns the sanitized bytes
// without writing anything to disk: the app layer copies the result
// into the outgoing media area itself. A pasted image comes from
// whichever other program last put something on the clipboard, so it
// is checked against the same full-size profile a downloaded photo
// is, never trusted just because the user chose to paste it.
func SanitizeOutgoingImage(data []byte) ([]byte, error) {
	safe, ok := imagesafe.Sanitize(data, imagesafe.FullImage)
	if !ok {
		return nil, fmt.Errorf("connector: sanitize outgoing image: %w", ErrUnsafeImage)
	}

	return safe, nil
}

// SaveMedia writes data to path unchanged, readable only by the
// user, for a video, audio, voice note, file or document: formats
// Qt's own image decoders never touch, so none of SaveImageMedia's
// risk applies.
func SaveMedia(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("connector: save media: %w", err)
	}

	return nil
}
