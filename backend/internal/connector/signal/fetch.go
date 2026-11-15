package signal

import (
	"context"
	"fmt"
	"os"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// rememberAttachment saves ptr as the attachment behind
// conversationRemoteID's messageRemoteID, for a later FetchMedia.
// Best effort, as WhatsApp's saveMediaRef is; a nil ptr is a no-op.
func (c *Connector) rememberAttachment(ctx context.Context, conversationRemoteID, messageRemoteID string, ptr *signalpb.AttachmentPointer) {
	if ptr == nil {
		return
	}

	media := c.mediaFor()
	if media == nil {
		return
	}

	_ = media.putAttachment(ctx, conversationRemoteID, messageRemoteID, ptr, nil)
}

// attachmentFor returns the attachment pointer saved for
// conversationRemoteID's messageRemoteID and its plaintext hash, when
// that is how a backup history import verified it (see
// backupmedia.go), or a nil pointer when none was saved.
func (c *Connector) attachmentFor(ctx context.Context, conversationRemoteID, messageRemoteID string) (*signalpb.AttachmentPointer, []byte) {
	media := c.mediaFor()
	if media == nil {
		return nil, nil
	}

	ptr, plaintextHash, found, err := media.attachment(ctx, conversationRemoteID, messageRemoteID)
	if err != nil || !found {
		return nil, nil
	}

	return ptr, plaintextHash
}

// FetchMedia downloads and decrypts messageRemoteID's attachment to
// path. auto is unused: Signal attachments need no separate limit.
// A message imported from history with no transit-tier copy at all
// (older or media-tier-only backups) was never remembered with a
// pointer, so it reports the same "not yet implemented" error as an
// unknown message, rather than panicking on a nil pointer.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, _ bool) error {
	ptr, plaintextHash := c.attachmentFor(ctx, conv.RemoteID, messageRemoteID)
	if ptr == nil {
		return fmt.Errorf("signal: fetch media: %w", errNotImplemented)
	}

	dev, _, err := c.session()
	if err != nil {
		return err
	}

	// Read-write: signalmeow reads the download back to decrypt it in place.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("signal: fetch media: %w", err)
	}
	defer func() { _ = file.Close() }() // the download below is what matters; a close failure changes nothing

	if err := dev.downloadAttachment(ctx, ptr, plaintextHash, file); err != nil {
		return fmt.Errorf("signal: fetch media: %w", err)
	}

	return nil
}
