package signal

import (
	"context"
	"fmt"
	"os"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// maxTrackedAttachments bounds how many messages' attachment pointers
// one run remembers for a later FetchMedia, the same way the poll and
// reaction tables are bounded. An attachment received in an earlier
// run is not fetchable again after a restart: a known limitation, see
// docs/decisions.md.
const maxTrackedAttachments = 1024

// rememberAttachment records ptr as the attachment behind
// conversationRemoteID's messageRemoteID, for FetchMedia, evicting the
// oldest remembered attachment first once this run is already at
// maxTrackedAttachments. A nil ptr is a no-op: most messages carry no
// fetchable attachment at all.
func (c *Connector) rememberAttachment(conversationRemoteID, messageRemoteID string, ptr *signalpb.AttachmentPointer) {
	if ptr == nil {
		return
	}

	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	key := conversationRemoteID + "/" + messageRemoteID
	if c.attachments == nil {
		c.attachments = map[string]*signalpb.AttachmentPointer{}
	}
	if _, exists := c.attachments[key]; !exists && len(c.attachments) >= maxTrackedAttachments {
		oldest, rest := c.attachmentOrder[0], c.attachmentOrder[1:]
		delete(c.attachments, oldest)
		c.attachmentOrder = rest
	}
	if _, exists := c.attachments[key]; !exists {
		c.attachmentOrder = append(c.attachmentOrder, key)
	}
	c.attachments[key] = ptr
}

// attachmentFor returns the attachment pointer remembered for
// conversationRemoteID's messageRemoteID, or nil when none was.
func (c *Connector) attachmentFor(conversationRemoteID, messageRemoteID string) *signalpb.AttachmentPointer {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	return c.attachments[conversationRemoteID+"/"+messageRemoteID]
}

// FetchMedia downloads and decrypts messageRemoteID's attachment to
// path. auto is accepted to satisfy connector.MediaFetcher; Signal
// attachments are never large enough to need a different limit for an
// automatic fetch than for one the user asked for (see limits.go in
// the WhatsApp connector for the contrast), so it is otherwise unused.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, _ bool) error {
	ptr := c.attachmentFor(conv.RemoteID, messageRemoteID)
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
