package whatsapp

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// isReaction reports whether msg is a reaction to another message
// rather than content of its own.
func isReaction(msg *waE2E.Message) bool {
	return msg.GetReactionMessage() != nil
}

// reaction reads which message msg reacts to and which emoji, or ""
// when it clears a previous reaction. WhatsApp reports one person's
// reaction at a time, not a conversation's full tally, so the connector
// folds this single change into that message's reaction list itself.
func reaction(msg *waE2E.Message) (messageRemoteID, emoji string) {
	r := msg.GetReactionMessage()
	return r.GetKey().GetID(), r.GetText()
}

// isRevoke reports whether msg deletes another message rather than
// being content of its own. ProtocolMessage_REVOKE is zero, the enum's
// default value, so a message with no protocol message at all must be
// ruled out explicitly, or every plain message would look like one.
func isRevoke(msg *waE2E.Message) bool {
	pm := msg.GetProtocolMessage()
	return pm != nil && pm.GetType() == waE2E.ProtocolMessage_REVOKE
}

// revoke reads which message a revoke protocol message deletes.
func revoke(msg *waE2E.Message) (messageRemoteID string) {
	return msg.GetProtocolMessage().GetKey().GetID()
}

// isEdit reports whether msg replaces the content of another message
// rather than being content of its own.
func isEdit(msg *waE2E.Message) bool {
	pm := msg.GetProtocolMessage()
	return pm != nil && pm.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT
}

// edit turns a message-edit protocol message into the message it
// replaces, with its new text, media and reply: WhatsApp resends the
// whole message rather than a diff, so Sink.Edited's "replace
// unconditionally" contract fits exactly.
func edit(info types.MessageInfo, msg *waE2E.Message) domain.Message {
	pm := msg.GetProtocolMessage()
	out := message(info, pm.GetEditedMessage())
	out.RemoteID, out.Edited = pm.GetKey().GetID(), true

	return out
}

// receiptStatus maps a WhatsApp receipt type to a domain delivery
// status, or "" for a receipt kind the UI has no status for, such as a
// retry request or a view-once "played" notice.
func receiptStatus(t types.ReceiptType) string {
	switch t {
	case types.ReceiptTypeDelivered, types.ReceiptTypeSender:
		return domain.StatusDelivered
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		return domain.StatusRead
	default:
		return ""
	}
}

// typingActive reports whether a chat presence update means someone
// started typing, as opposed to pausing.
func typingActive(state types.ChatPresence) bool {
	return state == types.ChatPresenceComposing
}
