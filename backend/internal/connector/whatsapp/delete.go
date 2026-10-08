package whatsapp

// delete.go covers both directions of deleting a message: handleDeleteForMe
// reports a message the phone deleted "for me" on another linked device
// (WhatsApp's events.DeleteForMe), the incoming counterpart to a live
// revoke's handleRevoke in live.go; DeleteMessages implements
// connector.Deleter, this account asking WhatsApp to delete a message
// itself, for everyone with a revoke or only for this account with the
// same app-state patch WhatsApp's own app sends for "delete for me".

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Deleter = (*Connector)(nil)

// deleteTimeout bounds how long DeleteMessages waits for WhatsApp to
// accept one message's revoke or delete-for-me patch, matching this
// connector's other per-message calls such as React.
const deleteTimeout = 30 * time.Second

// errCannotRevoke reports "for everyone" asked of a message this
// account did not send, or one this connector never saved the sender
// of: WhatsApp only lets the sender revoke their own message (an admin
// revoking someone else's in a group is out of scope here), and
// without a saved key there is nothing to build a revoke's key from.
var errCannotRevoke = fmt.Errorf("whatsapp: delete: %w: only your own messages can be deleted for everyone", connector.ErrDeleteUnsupported)

// handleDeleteForMe reports a message removed from one of this
// account's conversations after it was deleted "for me" on another of
// its linked devices: a local deletion WhatsApp's own app-state sync
// replays to every device, this one included, never telling whoever
// sent it.
func (c *Connector) handleDeleteForMe(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.DeleteForMe) {
	remote := chatID(ctx, dev, media, e.ChatJID)
	sink.Deleted(ctx, c.account.ID, []string{remote}, []string{e.MessageID})
}

// DeleteMessages deletes each of remoteIDs from conv: for everyone, by
// revoking a message this account sent, or only for this account, with
// an app-state patch, for any message regardless of who sent it.
func (c *Connector) DeleteMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: delete: %w", err)
	}

	media := c.mediaFor()
	for _, id := range remoteIDs {
		key, found, err := messageKeyFor(ctx, media, conv.RemoteID, id)
		if err != nil {
			return fmt.Errorf("whatsapp: delete: %w", err)
		}

		if forEveryone && (!found || !key.fromMe) {
			return errCannotRevoke
		}

		if forEveryone {
			err = sendRevoke(ctx, dev, chat, key, id)
		} else {
			err = deleteForMe(ctx, dev, chat, key, id)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// messageKeyFor looks up messageRemoteID's saved key, or the zero key
// when media is nil, such as a connector built without one in a test.
func messageKeyFor(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string) (messageKey, bool, error) {
	if media == nil {
		return messageKey{}, false, nil
	}

	return media.messageKeyFor(ctx, conversationRemoteID, messageRemoteID)
}

// sendRevoke asks WhatsApp to delete a message this account sent, for
// everyone. The caller has already checked that key names a message
// this account actually sent; sendRevoke only builds and sends the
// protocol message for it. Named apart from normalize_events.go's own
// revoke, which instead reads which message an incoming revoke deletes.
func sendRevoke(ctx context.Context, dev device, chat types.JID, key messageKey, messageRemoteID string) error {
	revokeCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: targetKey(chat, key, messageRemoteID),
	}}
	if _, err := dev.sendMessage(revokeCtx, chat, msg, dev.generateMessageID()); err != nil {
		return fmt.Errorf("whatsapp: delete: %w", err)
	}

	return nil
}

// deleteForMe removes a message from this account's own view of chat,
// on every linked device, without telling whoever sent it.
func deleteForMe(ctx context.Context, dev device, chat types.JID, key messageKey, messageRemoteID string) error {
	patchCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	patch := buildDeleteForMe(chat, messageRemoteID, key.fromMe, key.senderID)
	if err := dev.sendAppState(patchCtx, patch); err != nil {
		return fmt.Errorf("whatsapp: delete: %w", err)
	}

	return nil
}

// buildDeleteForMe builds the app-state patch WhatsApp's own app sends
// for "delete for me": every other linked device replays it as the
// events.DeleteForMe this connector already reports (see
// handleDeleteForMe). whatsmeow exposes no BuildDeleteForMe helper for
// this pinned version, only the mutation's index name
// (appstate.IndexDeleteMessageForMe) and proto message, so the patch is
// built the same way its own BuildStar does for another per-message,
// per-sender app-state action: chat, message id, a "1"/"0" from-me flag
// and, for anyone else, the sender, as positional strings in the
// mutation's index, with "0" standing in for "no distinct sender" the
// same way BuildStar's own fallback does.
func buildDeleteForMe(chat types.JID, messageID string, fromMe bool, senderID string) appstate.PatchInfo {
	isFromMe := "0"
	if fromMe {
		isFromMe = "1"
	}

	sender := senderID
	if fromMe || sender == "" {
		sender = "0"
	}

	return appstate.PatchInfo{
		Type: appstate.WAPatchRegularHigh,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexDeleteMessageForMe, chat.String(), messageID, isFromMe, sender},
			Version: 2,
			Value: &waSyncAction.SyncActionValue{
				DeleteMessageForMeAction: &waSyncAction.DeleteMessageForMeAction{
					DeleteMedia:      boolp(false),
					MessageTimestamp: int64p(time.Now().UnixMilli()),
				},
			},
		}},
	}
}
