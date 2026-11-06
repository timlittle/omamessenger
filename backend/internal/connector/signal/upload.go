package signal

// upload.go sends photos, videos, files and voice notes: Send uploads
// an outgoing message's attachment with signalmeow and fills in the
// pointer fields a DataMessage's attachment list needs beyond what
// UploadAttachment itself returns. Unlike WhatsApp, Signal carries
// every attachment kind as the same AttachmentPointer shape, so there
// is no separate image, video or document submessage to build.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errUnsupportedAttachment reports an attachment of a kind this
// connector cannot send: stickers and polls are built and sent through
// their own DataMessage fields, never as an attachment.
var errUnsupportedAttachment = errors.New("signal: sending this kind of attachment is not supported")

// uploadAttachment uploads m's attachment to Signal's media servers
// and returns the pointer its DataMessage attaches it with, carrying
// the content type, file name and, for a voice note, the flag Signal's
// own clients use to show a waveform player instead of a file icon.
func uploadAttachment(ctx context.Context, dev device, m domain.Message) (*signalpb.AttachmentPointer, error) {
	if err := checkAttachmentKind(m.Media.Kind); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(m.Media.Path)
	if err != nil {
		return nil, fmt.Errorf("signal: send: %w", err)
	}

	ptr, err := dev.uploadAttachment(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("signal: send: upload: %w", err)
	}

	ptr.ContentType = strp(sniffMimeType(data))
	ptr.FileName = strp(m.Media.FileName)
	if m.Media.Kind == domain.MediaVoice {
		ptr.Flags = u32p(uint32(signalpb.AttachmentPointer_VOICE_MESSAGE))
	}

	return ptr, nil
}

// checkAttachmentKind reports errUnsupportedAttachment for a media
// kind this connector cannot send as an attachment.
func checkAttachmentKind(kind string) error {
	switch kind {
	case domain.MediaPhoto, domain.MediaVideo, domain.MediaFile, domain.MediaVoice:
		return nil
	default:
		return fmt.Errorf("%w: %s", errUnsupportedAttachment, kind)
	}
}

// sniffMimeType detects a file's content type from its first bytes.
func sniffMimeType(data []byte) string {
	n := min(len(data), 512)

	return http.DetectContentType(data[:n])
}
