package whatsapp

// limits.go sizes an outgoing attachment before it is uploaded.
// WhatsApp caps media (photo/video) at 16 MiB and documents at 2 GiB;
// this connector always sends photos and videos as media, never as documents.

import (
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mediaAttachmentLimit is the largest photo or video WhatsApp accepts
// sent as media, rather than as a document.
const mediaAttachmentLimit int64 = 16 << 20

// Connector satisfies connector.AttachmentLimiter.
var _ connector.AttachmentLimiter = (*Connector)(nil)

// documentAttachmentLimit is the largest file WhatsApp accepts sent
// through the document picker.
const documentAttachmentLimit int64 = 2 << 30

// AttachmentLimit reports the largest attachment of kind this account can send.
func (*Connector) AttachmentLimit(kind string) int64 {
	switch kind {
	case domain.MediaPhoto, domain.MediaVideo:
		return mediaAttachmentLimit
	default:
		return documentAttachmentLimit
	}
}
