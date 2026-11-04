package connector

import (
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// ErrUnsafeImage reports that a photo or sticker failed imagesafe's
// check. The app layer reports this the same way as any other failed
// fetch: the UI falls back to a thumbnail or a placeholder, never a
// crash.
var ErrUnsafeImage = errors.New("connector: image failed safety check")

// SanitizeOutgoingImage re-encodes data, an image the user is
// attaching to an outgoing message, through imagesafe.Reencode under
// the Outgoing profile, and returns the result without writing to
// disk. A pasted image is never trusted just because the user chose
// to paste it (see docs/decisions.md).
func SanitizeOutgoingImage(data []byte) ([]byte, error) {
	safe, _, _, _, ok := imagesafe.Reencode(data, imagesafe.Outgoing)
	if !ok {
		return nil, fmt.Errorf("connector: sanitize outgoing image: %w", ErrUnsafeImage)
	}

	return safe, nil
}

// SanitizeOutgoingImageFile overwrites path in place with
// SanitizeOutgoingImage's re-encoded rewrite, for a photo attachment
// the app layer copied in from disk. path's declared size is checked
// against the profile's input cap before it is read into memory.
func SanitizeOutgoingImageFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}
	if info.Size() > imagesafe.Outgoing.MaxInputLen {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", ErrMediaTooLarge)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	safe, err := SanitizeOutgoingImage(data)
	if err != nil {
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	if err := os.WriteFile(path, safe, 0o600); err != nil { //nolint:gosec // deliberate: path is the outgoing media area path the app layer already wrote this same file to; it is never built from untrusted input
		return fmt.Errorf("connector: sanitize outgoing image file: %w", err)
	}

	return nil
}

// SaveMedia writes data to path unchanged, readable only by the user.
// Every connector's FetchMedia uses this for video, audio, voice notes
// and files, and now for a photo or sticker's own original too: its
// display copy is what the UI actually decodes (see ReencodeImageFile
// and docs/decisions.md).
func SaveMedia(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("connector: save media: %w", err)
	}

	return nil
}

// ReencodeImageFile reads srcPath fully through imagesafe.Reencode
// under profile and writes the result to dstPath, readable only by
// the user, so Qt's own decoders only ever see bytes this encoder
// wrote (see docs/decisions.md). Called once to fill a display copy's
// cache slot; format, width and height are Reencode's own result, for
// naming and sizing that slot.
func ReencodeImageFile(srcPath, dstPath string, profile imagesafe.Profile) (format imagesafe.OutputFormat, width, height int, err error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}
	if info.Size() > profile.MaxInputLen {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", ErrMediaTooLarge)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}

	out, format, width, height, ok := imagesafe.Reencode(data, profile)
	if !ok {
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", ErrUnsafeImage)
	}

	if err := os.WriteFile(dstPath, out, 0o600); err != nil { //nolint:gosec // deliberate: dstPath is a display copy's cache path app.Commands already builds through cache.FetchNamed, never built from untrusted input
		return imagesafe.FormatUnknown, 0, 0, fmt.Errorf("connector: reencode image file: %w", err)
	}

	return format, width, height, nil
}
