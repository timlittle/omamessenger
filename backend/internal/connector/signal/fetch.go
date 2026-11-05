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

	_ = media.putAttachment(ctx, conversationRemoteID, messageRemoteID, ptr)
}

// attachmentFor returns the attachment pointer saved for
// conversationRemoteID's messageRemoteID, or nil when none was.
func (c *Connector) attachmentFor(ctx context.Context, conversationRemoteID, messageRemoteID string) *signalpb.AttachmentPointer {
	media := c.mediaFor()
	if media == nil {
		return nil
	}

	ptr, found, err := media.attachment(ctx, conversationRemoteID, messageRemoteID)
	if err != nil || !found {
		return nil
	}

	return ptr
}

// FetchMedia downloads and decrypts messageRemoteID's attachment to
// path. auto is unused: Signal attachments need no separate limit.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, _ bool) error {
	ptr := c.attachmentFor(ctx, conv.RemoteID, messageRemoteID)
	if ptr == nil {
		return fmt.Errorf("signal: fetch media: %w", errNotImplemented)
	}

	dev, _, err := c.session()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("signal: fetch media: %w", err)
	}
	defer func() { _ = file.Close() }() // the download below is what matters; a close failure changes nothing

	if err := dev.downloadAttachment(ctx, ptr, file); err != nil {
		return fmt.Errorf("signal: fetch media: %w", err)
	}

	return nil
}
