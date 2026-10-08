package whatsapp

// markread.go tells WhatsApp a conversation has been read here: a read
// receipt for each of its newest unread incoming messages, grouped by
// sender, and the app-state mutation that marks the whole chat read,
// which is what actually updates the phone's own badge and any other
// linked device's. Both are read back from the message_keys this
// connector persists (see keys.go) rather than from anything tracked
// only in this process's memory, so MarkRead still works right after a
// restart or a re-pair, when nothing has been read live yet: a
// conversation whose unread messages all arrived in an earlier run, or
// during history sync, is marked read exactly the same way as one whose
// messages just arrived live.

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// readTimeout bounds how long MarkRead waits for WhatsApp to accept a
// read receipt or the chat-read app-state patch.
const readTimeout = 30 * time.Second

// MarkRead tells WhatsApp the newest conv.Unread incoming messages of
// conv have been read: one receipt call per sender, since WhatsApp only
// accepts message ids from a single sender in one call (in a group,
// that sender is the participant who sent them), followed by the
// app-state mutation that marks the whole chat read. It does nothing
// when this connector has no media store to read message keys from, or
// when conv.Unread is 0 or message_keys has nothing saved for it, the
// same no-op either case produced before.
func (c *Connector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil {
		return nil
	}

	pending, err := media.unreadMessageKeys(ctx, conv.RemoteID, conv.Unread)
	if err != nil {
		return fmt.Errorf("whatsapp: mark read: %w", err)
	}
	if len(pending) == 0 {
		logMarkRead(0, 0, chatPatchResult{status: chatPatchSkipped})
		return nil
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: mark read: %w", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	receipts := 0
	for senderRemoteID, ids := range pending {
		if err := markReadFrom(readCtx, dev, chat, senderRemoteID, ids); err != nil {
			return err
		}
		receipts += len(ids)
	}

	patch := sendChatReadState(readCtx, dev, media, chat, conv.RemoteID)
	logMarkRead(receipts, len(pending), patch)

	return nil
}

// markReadFrom tells WhatsApp the messages ids, all sent by the
// participant named senderRemoteID, have been read, skipping a sender id
// this connector did not make rather than failing the whole call.
func markReadFrom(ctx context.Context, dev device, chat types.JID, senderRemoteID string, ids []string) error {
	sender, err := jidFromRemoteID(senderRemoteID)
	if err != nil {
		return nil
	}

	messageIDs := make([]types.MessageID, len(ids))
	for i, id := range ids {
		messageIDs[i] = types.MessageID(id)
	}

	if err := dev.markRead(ctx, messageIDs, chat, sender); err != nil {
		return fmt.Errorf("whatsapp: mark read: %w", err)
	}

	return nil
}

// The chat_patch values logMarkRead reports for sendChatReadState's
// outcome: sent once WhatsApp accepted the mutation, failed once it
// did not, and skipped when there was no saved message to name it by
// at all.
const (
	chatPatchSent    = "sent"
	chatPatchFailed  = "failed"
	chatPatchSkipped = "skipped"
)

// chatPatchResult is what sendChatReadState's app-state mutation
// achieved, for logMarkRead's diagnostic line: its outcome, and whether
// the message it named had a real timestamp rather than the zero value
// an account paired before keys.go's timestamp column existed would
// still carry for a message never seen again since.
type chatPatchResult struct {
	status       string
	hasTimestamp bool
}

// sendChatReadState sends the app-state mutation that marks chat fully
// read, naming the newest message this connector has saved for it, so
// WhatsApp's own servers, the phone and any other linked device agree
// the chat is read up to that point; this is what actually clears the
// badge WhatsApp itself shows, which the per-message receipts
// markReadFrom already sent do not touch on their own. It is best
// effort: a failure here is reported through the returned result rather
// than failing the whole MarkRead call, the same as Telegram's own read
// reporting, since the per-message receipts already sent still update
// WhatsApp's server-side progress either way, and a later resync
// corrects any miss.
func sendChatReadState(ctx context.Context, dev device, media *mediaStore, chat types.JID, convRemoteID string) chatPatchResult {
	messageID, key, found, err := media.latestMessageKey(ctx, convRemoteID)
	if err != nil || !found {
		return chatPatchResult{status: chatPatchSkipped}
	}

	result := chatPatchResult{status: chatPatchSent, hasTimestamp: key.timestamp != 0}

	patch := appstate.BuildMarkChatAsRead(chat, true, time.UnixMilli(key.timestamp), targetKey(chat, key, messageID))
	if err := dev.sendAppState(ctx, patch); err != nil {
		result.status = chatPatchFailed
	}

	return result
}

// logMarkRead reports one MarkRead call's shape: how many read receipts
// it sent and across how many senders, whether the app-state mutation
// marking the whole chat read was sent, failed, or never attempted at
// all (see chatPatchResult), and whether the message that mutation
// named carried a real timestamp. A report of "still shows unread on
// the phone" can be checked against this: receipts and senders both 0
// means nothing was unread by this connector's own reckoning in the
// first place, not that sending failed.
func logMarkRead(receipts, senders int, patch chatPatchResult) {
	log.Printf("whatsapp: mark read (receipts=%d, senders=%d, chat_patch=%s, has_timestamp=%v)",
		receipts, senders, patch.status, patch.hasTimestamp)
}
