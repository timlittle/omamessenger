package whatsapp

// edit.go implements connector.MessageEditor: changing the text of a
// message this account already sent, by sending WhatsApp's own edit
// protocol message (whatsmeow.Client.BuildEdit's shape, built by hand
// here since the device interface, not *whatsmeow.Client itself, is
// what this connector drives - see device.go). Only a message this
// account sent, saved with its key, can be edited, and only within
// WhatsApp's own edit window.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.MessageEditor = (*Connector)(nil)

// editTimeout bounds how long EditMessage waits for WhatsApp to accept
// one message's edit, matching this connector's other per-message calls
// such as React and DeleteMessages.
const editTimeout = 30 * time.Second

// errCannotEdit reports "edit" asked of a message this account did not
// send, or one this connector never saved the key of: WhatsApp only lets
// the sender edit their own message, and without a saved key there is
// nothing to build the edit's target key from.
var errCannotEdit = fmt.Errorf("whatsapp: edit: %w: only your own messages can be edited", connector.ErrEditUnsupported)

// errEditExpired reports a message past WhatsApp's own time limit to
// edit it.
var errEditExpired = fmt.Errorf("whatsapp: edit: %w: the time to edit this message has passed", connector.ErrEditUnsupported)

// EditMessage changes the text of the message remoteID names in conv to
// newText, reapplying mentions the same way a plain outgoing message
// does.
func (c *Connector) EditMessage(ctx context.Context, conv domain.Conversation, remoteID, newText string, mentions []domain.Mention) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: edit: %w", err)
	}

	key, err := editableKey(ctx, c.mediaFor(), conv.RemoteID, remoteID)
	if err != nil {
		return err
	}

	text, mentionedJIDs := outgoingMentions(newText, mentions)
	content := plainOrMentionedMessage(text, mentionedJIDs)
	msg := buildEdit(chat, key, remoteID, content)

	return sendEdit(ctx, dev, chat, msg)
}

// editableKey looks up remoteID's saved key and reports whether it can
// be edited: known, sent by this account, and still inside WhatsApp's
// edit window.
func editableKey(ctx context.Context, media *mediaStore, conversationRemoteID, remoteID string) (messageKey, error) {
	key, found, err := messageKeyFor(ctx, media, conversationRemoteID, remoteID)
	if err != nil {
		return messageKey{}, fmt.Errorf("whatsapp: edit: %w", err)
	}
	if !found || !key.fromMe {
		return messageKey{}, errCannotEdit
	}
	if key.timestamp != 0 && time.Since(time.UnixMilli(key.timestamp)) > whatsmeow.EditWindow {
		return messageKey{}, errEditExpired
	}

	return key, nil
}

// sendEdit sends msg, an edit protocol message buildEdit already built,
// to chat.
func sendEdit(ctx context.Context, dev device, chat types.JID, msg *waE2E.Message) error {
	editCtx, cancel := context.WithTimeout(ctx, editTimeout)
	defer cancel()

	if _, err := dev.sendMessage(editCtx, chat, msg, dev.generateMessageID()); err != nil {
		return fmt.Errorf("whatsapp: edit: %w", err)
	}

	return nil
}

// buildEdit builds the protocol message WhatsApp's own clients send to
// edit remoteID's content: the same shape whatsmeow.Client.BuildEdit
// builds, reusing targetKey (see keys.go) for the message key, since an
// editable key is always this account's own (fromMe), the one case
// targetKey never adds a participant for.
func buildEdit(chat types.JID, key messageKey, remoteID string, newContent *waE2E.Message) *waE2E.Message {
	return &waE2E.Message{
		EditedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ProtocolMessage: &waE2E.ProtocolMessage{
					Key:           targetKey(chat, key, remoteID),
					Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
					EditedMessage: newContent,
					TimestampMS:   int64p(time.Now().UnixMilli()),
				},
			},
		},
	}
}
