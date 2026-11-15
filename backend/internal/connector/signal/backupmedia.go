package signal

// backupmedia.go maps a transferred history message's attachment or
// link preview to the same domain.Media shape a live message's own
// normalizer builds (see normalize.go), and remembers its transit
// pointer for a later FetchMedia the way a live attachment is.
// Without this, a backup message carrying only media and no text body
// was dropped entirely, and a captioned one lost its media.

import (
	"context"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// applyBackupMedia fills m's Media from std's first attachment or
// link preview, matching applyMedia's own live precedence, and
// returns the chosen attachment's own backup wrapper for a later
// FetchMedia, or nil when std carries no attachment. A caption lives
// in either the message's own text body or the attachment's pointer;
// the body wins when both are set, so the result is always one
// message carrying both its text and its media, never two. Every
// stored message needs text (store.ErrInvalidMessage), so one with
// media and no caption anywhere gets the same placeholder a connector
// gives an outgoing captionless attachment (domain.MediaPlaceholder).
func applyBackupMedia(m *domain.Message, std *backuppb.StandardMessage) *backuppb.MessageAttachment {
	var att *backuppb.MessageAttachment

	switch {
	case len(std.GetAttachments()) > 0:
		att = std.GetAttachments()[0]
		m.Media = backupAttachmentMedia(att)
		if m.Text == "" {
			m.Text = att.GetPointer().GetCaption()
		}
	case len(std.GetLinkPreview()) > 0:
		m.Media = backupPreviewMedia(std.GetLinkPreview()[0])
	default:
		return nil
	}

	if m.Text == "" {
		m.Text = domain.MediaPlaceholder(m.Media.Kind)
	}

	return att
}

// backupAttachmentMedia maps a backup attachment's pointer to a
// domain.Media, the same fields attachmentMedia reads off a live
// AttachmentPointer. Backups carry no inline thumbnail bytes (only a
// blur hash), so Thumb is left unset.
func backupAttachmentMedia(att *backuppb.MessageAttachment) *domain.Media {
	ptr := att.GetPointer()
	voice := att.GetFlag() == backuppb.MessageAttachment_VOICE_MESSAGE

	media := &domain.Media{
		Kind:     mediaKindForContentType(ptr.GetContentType(), voice),
		FileName: ptr.GetFileName(),
		Size:     int64(ptr.GetLocatorInfo().GetSize()),
		Width:    int(ptr.GetWidth()),
		Height:   int(ptr.GetHeight()),
	}

	if voice {
		media.Duration = int(ptr.GetAudioDurationSeconds())
	}
	if att.GetFlag() == backuppb.MessageAttachment_GIF {
		media.Animated = true
	}

	return media
}

// backupPreviewMedia maps a backup link preview, the same fields
// previewMedia reads off a live one.
func backupPreviewMedia(p *backuppb.LinkPreview) *domain.Media {
	url := p.GetUrl()
	if !domain.AllowedLinkURL(url) {
		url = ""
	}

	return &domain.Media{
		Kind:        domain.MediaFile,
		URL:         url,
		Title:       p.GetTitle(),
		Description: p.GetDescription(),
	}
}

// backupTransitPointer converts att's backup locator to the
// signalpb.AttachmentPointer FetchMedia downloads with, and its
// plaintext hash when that is the locator's own integrity check
// (newer backups hash the plaintext instead of keeping the sender's
// encrypted digest). found is false when the backup carries no
// transit-tier copy at all: an attachment stored only on the media
// tier, or one from history older than Signal's transit window, has
// no download path in signalmeow.
func backupTransitPointer(att *backuppb.MessageAttachment) (ptr *signalpb.AttachmentPointer, plaintextHash []byte, found bool) {
	fp := att.GetPointer()
	loc := fp.GetLocatorInfo()

	cdnKey := loc.GetTransitCdnKey()
	if cdnKey == "" {
		return nil, nil, false
	}

	ptr = &signalpb.AttachmentPointer{
		AttachmentIdentifier: &signalpb.AttachmentPointer_CdnKey{CdnKey: cdnKey},
		CdnNumber:            u32p(loc.GetTransitCdnNumber()),
		Key:                  loc.GetKey(),
		Size:                 u32p(loc.GetSize()),
		ContentType:          strp(fp.GetContentType()),
		FileName:             strp(fp.GetFileName()),
	}
	if digest := loc.GetEncryptedDigest(); len(digest) > 0 {
		ptr.Digest = digest
	}

	return ptr, loc.GetPlaintextHash(), true
}

// rememberBackupAttachment saves att's transit pointer the way
// rememberAttachment does for a live message, keeping its plaintext
// hash alongside when that is the backup's own integrity check. Best
// effort, as rememberAttachment is; a nil att, or one with no
// transit-tier copy, is a no-op, so FetchMedia later reports it
// unavailable the same way an unknown message does rather than ever
// seeing a half-built pointer.
func (c *Connector) rememberBackupAttachment(ctx context.Context, conversationRemoteID, messageRemoteID string, att *backuppb.MessageAttachment) {
	if att == nil {
		return
	}

	ptr, plaintextHash, ok := backupTransitPointer(att)
	if !ok {
		return
	}

	media := c.mediaFor()
	if media == nil {
		return
	}

	_ = media.putAttachment(ctx, conversationRemoteID, messageRemoteID, ptr, plaintextHash)
}
