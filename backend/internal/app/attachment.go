package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif" // registers GIF decoding for image.DecodeConfig
	"image/jpeg"
	_ "image/png" // registers PNG decoding for image.DecodeConfig
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

// thumbMaxSize bounds an outgoing photo's inline preview, in pixels on
// its longer side: large enough to recognise, small enough that sending
// it with every message costs nothing worth noticing.
const thumbMaxSize = 48

// prepareAttachment copies the file at path into the outgoing media
// area under id and describes it as a domain.Media: a photo or video for
// a kind the file's own bytes say it is, a file otherwise. An empty path
// means the message has no attachment, so it returns nil media and no
// error. When path names a real file the user picked from disk, rather
// than one already inside the outgoing area such as a clipboard paste,
// its path and modification time are remembered too, so a later retry
// can re-copy it if the outgoing copy ever goes missing.
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

	media := describeAttachment(stored, name, info.Size())
	if c.attachedFromDisk(path) {
		media.OriginalPath, media.OriginalModTime = path, info.ModTime().UnixMilli()
	}

	return media, nil
}

// attachedFromDisk reports whether path names a file outside the
// outgoing media area: one the user picked from disk, worth
// remembering for a retry to go back to, as opposed to an image
// PasteImage already copied in from the clipboard, which has no
// original of its own.
func (c *Commands) attachedFromDisk(path string) bool {
	if c.outgoing == nil {
		return false
	}

	return filepath.Dir(path) != filepath.Dir(c.outgoing.Path("x", "x"))
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
		media.Thumb = photoThumb(storedPath)
	}

	return media
}

// photoThumb returns a small base64 JPEG preview of the photo at path, so
// an outgoing photo's bubble has something to show before its own local
// copy loads, the same as an incoming photo's stripped preview. It
// returns "" when the file cannot be decoded, which only costs that
// moment: the full image still loads once sent.
func photoThumb(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close() // reading only; nothing was written to flush

	img, _, err := image.Decode(f)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, shrink(img, thumbMaxSize), &jpeg.Options{Quality: 60}); err != nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// shrink returns a nearest-neighbour copy of img no larger than limit on
// its longer side, cheap enough to run inline while sending; img itself
// when it is already within limit.
func shrink(img image.Image, limit int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= limit && h <= limit {
		return img
	}

	scale := float64(limit) / float64(max(w, h))
	nw, nh := scaledSize(w, scale), scaledSize(h, scale)

	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			out.Set(x, y, img.At(b.Min.X+x*w/nw, b.Min.Y+y*h/nh))
		}
	}

	return out
}

// scaledSize applies scale to n, never below one pixel.
func scaledSize(n int, scale float64) int {
	if s := int(float64(n) * scale); s > 0 {
		return s
	}

	return 1
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

// errAttachmentUnavailable reports a retry whose attachment cannot be
// found: its outgoing copy is gone, and either it has no original file
// to go back to, or that file no longer exists, or no longer looks like
// the one that was attached.
var errAttachmentUnavailable = fmt.Errorf("%w: the attachment is no longer available; attach it again", ErrInvalidInput)

// restoreAttachment makes sure m's outgoing copy exists on disk before
// a retry hands it to a connector, re-copying it from the original file
// it was attached from when the stored copy has gone missing and that
// file still looks like the one that was attached. It reads no file the
// user did not attach themselves, and reports errAttachmentUnavailable,
// a plain and safe message, when neither copy is available any more.
func (c *Commands) restoreAttachment(ctx context.Context, m *domain.Message) error {
	if m.Media == nil || c.outgoing == nil {
		return nil
	}

	path := c.outgoing.Path(m.ID, m.Media.FileName)
	if _, err := os.Stat(path); err == nil {
		m.Media.Path = path
		return nil
	}

	if err := c.recopyFromOriginal(ctx, m.ID, m.Media); err != nil {
		return err
	}

	m.Media.Path = path

	return nil
}

// recopyFromOriginal re-copies an attachment into the outgoing area
// under id from the file media says it was originally attached from,
// when that file still exists and still looks like the same one: same
// size and modification time. Anything else - no original to go back
// to, the file gone, or changed - reports errAttachmentUnavailable.
func (c *Commands) recopyFromOriginal(ctx context.Context, id string, media *domain.Media) error {
	if media.OriginalPath == "" {
		return errAttachmentUnavailable
	}

	info, err := os.Stat(media.OriginalPath)
	if err != nil || info.Size() != media.Size || info.ModTime().UnixMilli() != media.OriginalModTime {
		return errAttachmentUnavailable
	}

	if _, err := c.storeAttachment(ctx, id, media.FileName, media.OriginalPath); err != nil {
		return errAttachmentUnavailable
	}

	return nil
}
