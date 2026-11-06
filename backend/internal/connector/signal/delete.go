package signal

// delete.go implements connector.Deleter: deleting for everyone, by
// sending Signal's own DataMessage.Delete, within Signal's own time
// limit to do so. Signal has no protocol message for deleting a
// message only for this account: the phone never learns that a linked
// device removed its own local copy, so that case is left to the
// app's own local removal, with nothing sent here.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Deleter = (*Connector)(nil)

// deleteTimeout bounds how long DeleteMessages waits for Signal to
// accept one message's delete, matching this connector's other
// per-message calls.
const deleteTimeout = 30 * time.Second

// signalDeleteWindow is how long after sending a message Signal's own
// clients still let its sender delete it for everyone:
// https://support.signal.org/hc/en-us/articles/360050426432-Delete-for-everyone
const signalDeleteWindow = 24 * time.Hour

// errCannotDelete reports "for everyone" asked of a message this
// account did not send: Signal only lets the sender delete their own
// message for everyone.
var errCannotDelete = fmt.Errorf("signal: delete: %w: only your own messages can be deleted for everyone", connector.ErrDeleteUnsupported)

// errDeleteExpired reports a message past Signal's own time limit to
// delete it for everyone.
var errDeleteExpired = fmt.Errorf("signal: delete: %w: the time to delete this message for everyone has passed", connector.ErrDeleteUnsupported)

// DeleteMessages deletes each of remoteIDs from conv for everyone, or
// does nothing when forEveryone is false (see the doc comment above).
func (c *Connector) DeleteMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error {
	if !forEveryone {
		return nil
	}

	dev, _, err := c.session()
	if err != nil {
		return err
	}

	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	for _, id := range remoteIDs {
		if err := c.deleteForEveryone(deleteCtx, dev, conv.RemoteID, id); err != nil {
			return err
		}
	}

	return nil
}

// deleteForEveryone sends the Delete that removes messageRemoteID from
// every participant's own copy of conversationRemoteID, once it has
// checked self sent it and Signal's own window to do so has not
// passed.
func (c *Connector) deleteForEveryone(ctx context.Context, dev device, conversationRemoteID, messageRemoteID string) error {
	author, sentTimestamp, err := parseMessageRemoteID(messageRemoteID)
	if err != nil {
		return fmt.Errorf("signal: delete: %w", err)
	}
	if author != c.selfID() {
		return errCannotDelete
	}
	if time.Since(time.UnixMilli(int64(sentTimestamp))) > signalDeleteWindow {
		return errDeleteExpired
	}

	dm := &signalpb.DataMessage{
		Timestamp: u64p(uint64(time.Now().UnixMilli())),
		Delete:    &signalpb.DataMessage_Delete{TargetSentTimestamp: u64p(sentTimestamp)},
	}
	if err := sendToChat(ctx, dev, conversationRemoteID, signalmeow.WrapDataMessage(dm)); err != nil {
		return fmt.Errorf("signal: delete: %w", err)
	}

	return nil
}
