package whatsapp

import (
	"context"
	"slices"

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
func edit(ctx context.Context, dev device, info types.MessageInfo, msg *waE2E.Message) domain.Message {
	pm := msg.GetProtocolMessage()
	out := message(ctx, dev, info, pm.GetEditedMessage())
	out.RemoteID, out.Edited = pm.GetKey().GetID(), true

	return out
}

// isContentless reports whether msg is one of WhatsApp's own protocol
// or system notices rather than something a person sent: every
// ProtocolMessage kind other than a revoke or an edit (handled
// separately; see isRevoke and isEdit), a message pinned or kept in a
// chat, a voice or video call's log entry, an album's own header (its
// photos and videos arrive as their own messages, each wrapped in an
// associatedChildMessage that unwrap peels away; see
// normalize_message.go), or one of the other housekeeping kinds
// WhatsApp's wire format carries alongside a session (a history-sync
// bundle or notice, a secret or key-share payload), none of which carry
// anything a person actually said. A poll vote is handled separately
// too (see handlePollVote in vote.go), never replayed from a bulk sync
// the same way a reaction is not.
func isContentless(msg *waE2E.Message) bool {
	if pm := msg.GetProtocolMessage(); pm != nil {
		return !isRevoke(msg) && !isEdit(msg)
	}

	switch {
	case msg.GetPollUpdateMessage() != nil,
		msg.GetPinInChatMessage() != nil,
		msg.GetKeepInChatMessage() != nil,
		msg.GetCall() != nil,
		msg.GetCallLogMesssage() != nil,
		msg.GetBcallMessage() != nil,
		msg.GetAlbumMessage() != nil,
		msg.GetMessageHistoryBundle() != nil,
		msg.GetMessageHistoryNotice() != nil,
		msg.GetPlaceholderMessage() != nil,
		msg.GetSecretEncryptedMessage() != nil,
		msg.GetGroupRootKeyShare() != nil,
		msg.GetRootSecretDistributeMessage() != nil:
		return true
	default:
		return false
	}
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

// reactionTally turns one message's reactions, one per person who picked
// one, into the chips the UI shows: grouped by emoji, with how many
// people picked each one and whether selfKey is among them. WhatsApp
// reports one person's reaction change at a time, never a conversation's
// full tally, so the connector keeps bySender itself, keyed by who
// reacted, and recomputes this list on every change.
func reactionTally(bySender map[string]string, selfKey string) []domain.Reaction {
	counts := make(map[string]int, len(bySender))
	mine := make(map[string]bool, len(bySender))
	for sender, emoji := range bySender {
		counts[emoji]++
		if sender == selfKey {
			mine[emoji] = true
		}
	}

	emojis := make([]string, 0, len(counts))
	for emoji := range counts {
		emojis = append(emojis, emoji)
	}
	slices.Sort(emojis)

	reactions := make([]domain.Reaction, len(emojis))
	for i, emoji := range emojis {
		reactions[i] = domain.Reaction{Emoji: emoji, Count: counts[emoji], Mine: mine[emoji]}
	}

	return reactions
}

// typingActive reports whether a chat presence update means someone
// started typing, as opposed to pausing.
func typingActive(state types.ChatPresence) bool {
	return state == types.ChatPresenceComposing
}
