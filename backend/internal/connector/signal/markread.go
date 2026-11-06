package signal

// markread.go implements MarkRead: telling Signal a conversation has
// been read here, with one read receipt per sender (a group's unread
// messages can come from several), addressed to that sender's own
// account. signalmeow sends the matching read sync to this account's
// own other devices itself, as part of the same SendMessage call (see
// device.go's sendDirect), so the phone's own unread count clears too.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// readTimeout bounds how long MarkRead waits for Signal to accept
// each sender's read receipt.
const readTimeout = 30 * time.Second

// MarkRead tells Signal the newest conv.Unread incoming messages of
// conv have been read, from what this connector kept of them (see
// storage.go's recentIncoming). It does nothing when this connector
// has no media store, or conv has nothing unread by its own reckoning.
func (c *Connector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil || conv.Unread <= 0 {
		return nil
	}

	metas, err := media.recentIncoming(ctx, conv.RemoteID, conv.Unread)
	if err != nil {
		return fmt.Errorf("signal: mark read: %w", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	for sender, timestamps := range timestampsBySender(metas) {
		content := signalmeow.ReadReceptMessageForTimestamps(timestamps)
		if err := dev.sendDirect(readCtx, sender, content); err != nil {
			return fmt.Errorf("signal: mark read: %w", err)
		}
	}

	return nil
}

// timestampsBySender groups metas' own message ids by the sender
// each one's id names, dropping any id that does not parse.
func timestampsBySender(metas []incomingMeta) map[uuid.UUID][]uint64 {
	bySender := map[uuid.UUID][]uint64{}
	for _, meta := range metas {
		sender, timestamp, err := parseMessageRemoteID(meta.messageID)
		if err != nil {
			continue
		}

		bySender[sender] = append(bySender[sender], timestamp)
	}

	return bySender
}
