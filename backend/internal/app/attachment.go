package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"image"
	_ "image/gif"  // registers GIF decoding for image.DecodeConfig
	_ "image/jpeg" // registers JPEG decoding for image.DecodeConfig
	_ "image/png"  // registers PNG decoding for image.DecodeConfig
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// MaxAttachmentSize is the largest file Send accepts as an attachment.
// Telegram, the only service this helper sends media through today,
// refuses more than this for a non-Premium account.
const MaxAttachmentSize = 2 << 30

// prepareAttachment copies the file at path into the outgoing media
// area under id and describes it as a domain.Media: a photo or video for
// a kind the file's own bytes say it is, a file otherwise. An empty path
// means the message has no attachment, so it returns nil media and no
// error.
func (c *Commands) prepareAttachment(ctx context.Context, id, path string) (*domain.Media, error) {
	if path == "" {
		return nil, nil
	}

	if c.outgoing == nil {
		return nil, fmt.Errorf("%w: attachments are not available", ErrInvalidInput)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: the attached file could not be read", ErrInvalidInput)
	}

	if info.Size() > MaxAttachmentSize {
		return nil, fmt.Errorf("%w: files over 2 GB cannot be sent", ErrInvalidInput)
	}

	name := filepath.Base(path)
	stored, err := c.storeAttachment(ctx, id, name, path)
	if err != nil {
		return nil, err
	}

	return describeAttachment(stored, name, info.Size()), nil
}

// storeAttachment copies the file at path into the outgoing media area,
// opening it itself so prepareAttachment only deals with the result.
func (c *Commands) storeAttachment(ctx context.Context, id, name, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: the attached file could not be read", ErrInvalidInput)
	}

	stored, storeErr := c.outgoing.Store(ctx, id, name, f)
	closeErr := f.Close()

	switch {
	case storeErr != nil:
		return "", fmt.Errorf("store attachment: %w", storeErr)
	case closeErr != nil:
		return "", fmt.Errorf("store attachment: %w", closeErr)
	}

	return stored, nil
}

// describeAttachment sniffs a stored file's content to fill in a Media:
// photo or video for a common type its bytes say it is, file otherwise,
// with dimensions for a photo. name is the attachment's original file
// name, kept for display even though the stored copy's own name also
// carries the message id.
func describeAttachment(storedPath, name string, size int64) *domain.Media {
	kind := sniffKind(storedPath)
	media := &domain.Media{Kind: kind, Path: storedPath, FileName: name, Size: size}

	if kind == domain.MediaPhoto {
		media.Width, media.Height = imageDimensions(storedPath)
	}

	return media
}

// sniffKind reads a file's first bytes to say whether it is a photo, a
// video or, for anything else, a plain file.
func sniffKind(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return domain.MediaFile
	}
	defer f.Close() // reading only; a close failure changes nothing we already read

	buf := make([]byte, 512)
	n, _ := f.Read(buf) // a short or empty read still sniffs from what came back

	switch contentType := http.DetectContentType(buf[:n]); {
	case strings.HasPrefix(contentType, "image/"):
		return domain.MediaPhoto
	case strings.HasPrefix(contentType, "video/"):
		return domain.MediaVideo
	default:
		return domain.MediaFile
	}
}

// imageDimensions reads a photo's width and height without decoding the
// whole image, or (0, 0) if its format cannot be read.
func imageDimensions(path string) (width, height int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close() // reading only; dimensions are a bonus, not worth failing over

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}

	return cfg.Width, cfg.Height
}

// newAttachmentID returns a random id for a message carrying an
// attachment, generated before the message is stored so the same id
// names its file in the outgoing media area and finds it again on retry.
func newAttachmentID() string {
	return "m_" + rand.Text()
}
