package whatsapp

// upload.go sends photos, videos and files: Send uploads an outgoing
// message's attachment with whatsmeow and builds the image, video or
// document submessage that carries it, the way send.go already builds
// a plain or quoted-reply text message. The uploaded file's reference
// is saved the same way an incoming message's is (see
// normalize_media.go and storage.go), so FetchMedia can download this
// account's own sent copy back later, from another device signed into
// the same account.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // registers GIF decoding for image.DecodeConfig
	"image/jpeg"
	_ "image/png" // registers PNG decoding for image.DecodeConfig
	"net/http"
	"os"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errUnsupportedAttachment reports an attachment of a kind this
// connector cannot send: voice notes, ordinary audio files and
// stickers are received only (see normalize_media.go), since sending
// them is a later wave's job.
var errUnsupportedAttachment = errors.New("whatsapp: sending this kind of attachment is not supported")

// thumbMaxSize bounds an outgoing photo's JPEG thumbnail, in pixels on
// its longer side: large enough to recognise, small enough that
// sending it with every photo costs nothing worth noticing.
const thumbMaxSize = 48

// uploadAttachment uploads m's attachment to WhatsApp's media servers
// and returns the submessage that carries it, with m's caption and
// reply quoting.
func uploadAttachment(ctx context.Context, dev device, m domain.Message) (*waE2E.Message, error) {
	kind, err := attachmentKind(m.Media.Kind)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.Media.Path)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: send: %w", err)
	}

	resp, err := dev.uploadMedia(ctx, data, kind)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: send: upload: %w", err)
	}

	content := attachmentContent{
		data: data, mimetype: sniffMimeType(data), fileName: m.Media.FileName,
		caption: wireCaption(m), ctxInfo: replyContext(m.ReplyTo),
	}

	return attachmentMessage(kind, resp, content), nil
}

// attachmentKind says which whatsmeow media type an attachment of
// domain kind uploads and sends as, or errUnsupportedAttachment for a
// kind this connector cannot send.
func attachmentKind(kind string) (mediaKind, error) {
	switch kind {
	case domain.MediaPhoto:
		return mediaKindImage, nil
	case domain.MediaVideo:
		return mediaKindVideo, nil
	case domain.MediaFile:
		return mediaKindDocument, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedAttachment, kind)
	}
}

// attachmentContent is what an uploaded attachment needs besides its
// upload response to build the right kind of outgoing submessage: its
// bytes (image messages alone read these, for dimensions and a
// thumbnail), mimetype, file name, caption and reply quoting.
type attachmentContent struct {
	data     []byte
	mimetype string
	fileName string
	caption  string
	ctxInfo  *waE2E.ContextInfo
}

// attachmentMessage builds the WhatsApp submessage for an uploaded
// attachment of kind, from its upload response and content.
func attachmentMessage(kind mediaKind, resp whatsmeow.UploadResponse, content attachmentContent) *waE2E.Message {
	switch kind {
	case mediaKindImage:
		return &waE2E.Message{ImageMessage: imageMessage(resp, content)}
	case mediaKindVideo:
		return &waE2E.Message{VideoMessage: videoMessage(resp, content)}
	default:
		return &waE2E.Message{DocumentMessage: documentMessage(resp, content)}
	}
}

// imageMessage builds an outgoing image submessage: the uploaded
// reference, its mimetype, caption, reply quoting, dimensions and a
// generated JPEG thumbnail (whatsmeow carries no size probing of its
// own, the way Telegram's server fills in a photo's thumbnail for us).
func imageMessage(resp whatsmeow.UploadResponse, content attachmentContent) *waE2E.ImageMessage {
	w, h := imageDimensions(content.data)

	return &waE2E.ImageMessage{
		URL: strp(resp.URL), DirectPath: strp(resp.DirectPath), MediaKey: resp.MediaKey,
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256, FileLength: u64p(resp.FileLength),
		Mimetype: strp(content.mimetype), Caption: strp(content.caption), Width: u32p(uint32(w)), Height: u32p(uint32(h)),
		JPEGThumbnail: jpegThumbnail(content.data), ContextInfo: content.ctxInfo,
	}
}

// videoMessage builds an outgoing video submessage: the uploaded
// reference, its mimetype, caption and reply quoting. Dimensions and
// duration are left blank: reading them needs probing the container
// format, which the standard library cannot do without an external
// tool such as ffprobe, so WhatsApp just shows the video without them.
func videoMessage(resp whatsmeow.UploadResponse, content attachmentContent) *waE2E.VideoMessage {
	return &waE2E.VideoMessage{
		URL: strp(resp.URL), DirectPath: strp(resp.DirectPath), MediaKey: resp.MediaKey,
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256, FileLength: u64p(resp.FileLength),
		Mimetype: strp(content.mimetype), Caption: strp(content.caption), ContextInfo: content.ctxInfo,
	}
}

// documentMessage builds an outgoing document submessage: the uploaded
// reference, its mimetype, file name, caption and reply quoting.
func documentMessage(resp whatsmeow.UploadResponse, content attachmentContent) *waE2E.DocumentMessage {
	return &waE2E.DocumentMessage{
		URL: strp(resp.URL), DirectPath: strp(resp.DirectPath), MediaKey: resp.MediaKey,
		FileSHA256: resp.FileSHA256, FileEncSHA256: resp.FileEncSHA256, FileLength: u64p(resp.FileLength),
		Mimetype: strp(content.mimetype), Caption: strp(content.caption), FileName: strp(content.fileName), ContextInfo: content.ctxInfo,
	}
}

// sniffMimeType detects a file's content type from its first bytes.
func sniffMimeType(data []byte) string {
	n := min(len(data), 512)

	return http.DetectContentType(data[:n])
}

// imageDimensions reads a photo's width and height from its bytes
// without a full decode, or (0, 0) if its format cannot be read.
func imageDimensions(data []byte) (width, height int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}

	return cfg.Width, cfg.Height
}

// jpegThumbnail returns a small JPEG preview of a photo's bytes, no
// larger than thumbMaxSize on its longer side, for WhatsApp to show
// before the full image downloads. It returns nil when the photo
// cannot be decoded, which only costs that preview: the full image is
// still sent and can still be fetched.
func jpegThumbnail(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, shrinkThumbnail(img, thumbMaxSize), &jpeg.Options{Quality: 60}); err != nil {
		return nil
	}

	return buf.Bytes()
}

// shrinkThumbnail returns a nearest-neighbour copy of img no larger
// than limit on its longer side, or img itself when it is already
// within limit.
func shrinkThumbnail(img image.Image, limit int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= limit && h <= limit {
		return img
	}

	scale := float64(limit) / float64(max(w, h))
	nw, nh := scaledThumbSize(w, scale), scaledThumbSize(h, scale)

	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			out.Set(x, y, img.At(b.Min.X+x*w/nw, b.Min.Y+y*h/nh))
		}
	}

	return out
}

// scaledThumbSize applies scale to n, never below one pixel.
func scaledThumbSize(n int, scale float64) int {
	if s := int(float64(n) * scale); s > 0 {
		return s
	}

	return 1
}

// u32p takes the address of a uint32 value, for the generated protobuf
// structs that hold every optional field as a pointer.
func u32p(n uint32) *uint32 { return &n }

// u64p takes the address of a uint64 value, for the same reason.
func u64p(n uint64) *uint64 { return &n }
