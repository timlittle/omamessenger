package whatsapp

// limits.go sizes an outgoing attachment before it is uploaded.
// WhatsApp's own Help Center ("What's the file size limit for sending
// files on WhatsApp?",
// https://faq.whatsapp.com/1424709784556229) gives two figures: photos
// and videos sent as media are capped at 16 MiB, while a file sent
// through the document picker can reach 2 GiB. This connector always
// sends a photo or video as a native image or video message, never
// through the document picker (see upload.go), so the smaller media
// cap is the one that actually applies to those two kinds; a plain
// file keeps the larger document cap.

import "github.com/timlittle/omamessenger/backend/internal/domain"

// mediaAttachmentLimit is the largest photo or video WhatsApp accepts
// sent as media, rather than as a document.
const mediaAttachmentLimit int64 = 16 << 20

// documentAttachmentLimit is the largest file WhatsApp accepts sent
// through the document picker.
const documentAttachmentLimit int64 = 2 << 30

// AttachmentLimit reports the largest attachment of kind this account
// can send: mediaAttachmentLimit for a photo or video, which this
// connector always sends as media, or documentAttachmentLimit for
// anything else. The receiver is unused: WhatsApp's own cap depends
// only on kind, never on which account is asking.
func (*Connector) AttachmentLimit(kind string) int64 {
	switch kind {
	case domain.MediaPhoto, domain.MediaVideo:
		return mediaAttachmentLimit
	default:
		return documentAttachmentLimit
	}
}
