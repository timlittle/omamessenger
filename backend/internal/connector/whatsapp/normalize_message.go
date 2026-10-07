package whatsapp

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// unwrapDepth bounds how many wrapper layers unwrap peels, so a
// malformed or adversarially nested message can never loop forever.
const unwrapDepth = 8

// unwrap peels the ephemeral, view-once and own-device wrappers
// WhatsApp puts around a message's real content. whatsmeow's own event
// dispatch already does this before an events.Message reaches a
// connector, but normalize does it again so every function here is
// safe to call on a message straight from the wire, such as one read
// back out of a history sync.
func unwrap(msg *waE2E.Message) *waE2E.Message {
	for range unwrapDepth {
		switch {
		case msg.GetEphemeralMessage().GetMessage() != nil:
			msg = msg.GetEphemeralMessage().GetMessage()
		case msg.GetViewOnceMessage().GetMessage() != nil:
			msg = msg.GetViewOnceMessage().GetMessage()
		case msg.GetViewOnceMessageV2().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2().GetMessage()
		case msg.GetViewOnceMessageV2Extension().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2Extension().GetMessage()
		case msg.GetDeviceSentMessage().GetMessage() != nil:
			msg = msg.GetDeviceSentMessage().GetMessage()
		default:
			return msg
		}
	}

	return msg
}

// message turns a WhatsApp message into ours: its text, media, reply
// and the sender's current name. Outgoing messages, including ones
// WhatsApp reports from another of this account's own devices, are
// reported as already sent.
func message(ctx context.Context, dev device, info types.MessageInfo, raw *waE2E.Message) domain.Message {
	content := unwrap(raw)
	out := domain.Message{
		RemoteID: info.ID,
		Text:     messageText(content),
		Outgoing: info.IsFromMe,
		Status:   domain.StatusReceived,
		Created:  info.Timestamp.UnixMilli(),
		Media:    media(content),
		ReplyTo:  replyTo(contextInfo(content)),
	}

	if info.IsFromMe {
		out.SenderID, out.SenderName, out.Status = "self", "You", domain.StatusSent
		return out
	}

	out.SenderID, out.SenderName = remoteID(info.Sender), senderDisplayName(ctx, dev, info)
	return out
}

// historyMessage turns one message of a history sync conversation into
// ours, through the same message normalizer a live one uses, or reports
// false for a reaction, edit or revoke protocol message, or one of
// WhatsApp's other protocol or system notices (see isContentless): none
// of those carry content of their own to show as history, and the live
// events that cover a reaction, edit or revoke are never replayed from
// a bulk sync.
func historyMessage(ctx context.Context, dev device, chat types.JID, hm *waHistorySync.HistorySyncMsg) (domain.Message, bool) {
	raw := hm.GetMessage()
	content := raw.GetMessage()
	if content == nil || isReaction(content) || isRevoke(content) || isEdit(content) || isContentless(content) {
		return domain.Message{}, false
	}

	return message(ctx, dev, historyMessageInfo(chat, raw), content), true
}

// historyMessageInfo rebuilds the sender information a history sync's
// WebMessageInfo carries, in the same shape a live events.Message has,
// so both go through the same message normalizer.
func historyMessageInfo(chat types.JID, raw *waWeb.WebMessageInfo) types.MessageInfo {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: raw.GetKey().GetFromMe(), IsGroup: chat.Server == types.GroupServer},
		ID:            raw.GetKey().GetID(),
		PushName:      raw.GetPushName(),
		Timestamp:     time.Unix(int64(raw.GetMessageTimestamp()), 0),
	}

	if !info.IsFromMe {
		info.Sender = historySender(chat, raw)
	}

	return info
}

// historySender is who sent a history message we did not send ourselves:
// the chat itself for a direct message, or whichever participant
// WhatsApp recorded for a group one.
func historySender(chat types.JID, raw *waWeb.WebMessageInfo) types.JID {
	if chat.Server != types.GroupServer {
		return chat
	}

	participant := raw.GetParticipant()
	if participant == "" {
		participant = raw.GetKey().GetParticipant()
	}

	jid, _ := types.ParseJID(participant)

	return jid
}

// senderDisplayName names who sent a message: their resolved contact
// name, mapping a LID to its phone JID first (see device.contactName),
// or, failing that, whatever senderName can tell from the message
// itself. Resolving the contact first is what names a group's other
// members instead of the generic "WhatsApp user" once their contact or
// push name is known locally.
func senderDisplayName(ctx context.Context, dev device, info types.MessageInfo) string {
	if name := dev.contactName(ctx, info.Sender); name != "" {
		return name
	}

	return senderName(info)
}

// senderName names who sent a message: their self-chosen display name,
// their verified business name, or a generic label when WhatsApp
// reported neither. It never falls back to the phone number in their
// JID, which is not a name.
func senderName(info types.MessageInfo) string {
	if info.PushName != "" {
		return info.PushName
	}

	if name := verifiedName(info); name != "" {
		return name
	}

	return "WhatsApp user"
}

// verifiedName is a business account's verified name, or "" when the
// sender has none.
func verifiedName(info types.MessageInfo) string {
	if info.VerifiedName == nil || info.VerifiedName.Details == nil {
		return ""
	}

	return info.VerifiedName.Details.GetVerifiedName()
}

// contextInfo is the quoting and mention metadata carried by whichever
// kind of message content holds it, or nil for a plain text message or
// a kind with no such metadata.
func contextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage().GetContextInfo()
	default:
		return nil
	}
}

// replyTo reports the message ctx quotes, from its stanza id, or nil
// when it quotes nothing. The store fills in the quote's sender and
// excerpt from its own copy of that message, if it has one.
func replyTo(ctx *waE2E.ContextInfo) *domain.Reply {
	id := ctx.GetStanzaID()
	if id == "" {
		return nil
	}

	return &domain.Reply{RemoteID: id}
}

// messageText is a message's text, its caption, or a label for media
// without one, since every stored message has text.
func messageText(msg *waE2E.Message) string {
	if text := plainText(msg); text != "" {
		return text
	}

	if caption := mediaCaption(msg); caption != "" {
		return caption
	}

	return mediaPlaceholder(msg)
}

// plainText is a message's literal text: the body of an ordinary
// message, or one with a link preview. It is "" for anything else,
// including a caption, which mediaCaption reads instead.
func plainText(msg *waE2E.Message) string {
	switch {
	case msg.GetConversation() != "":
		return msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText()
	default:
		return ""
	}
}

// mediaCaption is the caption on a photo, video or file, or "" when the
// message carries no caption worth keeping.
func mediaCaption(msg *waE2E.Message) string {
	switch {
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetCaption()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}

// mediaPlaceholder labels a message that has neither plain text nor a
// caption, so every stored message still shows something.
func mediaPlaceholder(msg *waE2E.Message) string {
	switch {
	case msg.GetImageMessage() != nil:
		return "[Photo]"
	case msg.GetVideoMessage() != nil:
		return "[Video]"
	case msg.GetAudioMessage() != nil:
		return audioPlaceholder(msg.GetAudioMessage())
	case msg.GetDocumentMessage() != nil:
		return "[File]"
	case msg.GetStickerMessage() != nil:
		return "[Sticker]"
	default:
		if placeholder := sharedContentPlaceholder(msg); placeholder != "" {
			return placeholder
		}

		return "[Message]"
	}
}

// sharedContentPlaceholder labels a poll, a shared contact or a
// location, or "" for none of these: split out of mediaPlaceholder so
// neither function's own switch grows past this codebase's complexity
// limit.
func sharedContentPlaceholder(msg *waE2E.Message) string {
	switch {
	case pollCreation(msg) != nil:
		return pollPlaceholder(pollCreation(msg))
	case msg.GetContactMessage() != nil:
		return contactPlaceholder(msg.GetContactMessage().GetDisplayName())
	case msg.GetContactsArrayMessage() != nil:
		return "[Contact]"
	case msg.GetLocationMessage() != nil, msg.GetLiveLocationMessage() != nil:
		return "[Location]"
	default:
		return ""
	}
}

// pollCreation is the poll a message creates, whichever of WhatsApp's
// several poll message versions it was sent as, or nil when msg creates
// no poll.
func pollCreation(msg *waE2E.Message) *waE2E.PollCreationMessage {
	switch {
	case msg.GetPollCreationMessage() != nil:
		return msg.GetPollCreationMessage()
	case msg.GetPollCreationMessageV2() != nil:
		return msg.GetPollCreationMessageV2()
	case msg.GetPollCreationMessageV3() != nil:
		return msg.GetPollCreationMessageV3()
	default:
		return nil
	}
}

// pollPlaceholder labels a poll with its question, or just names it a
// poll when WhatsApp reported no question text.
func pollPlaceholder(p *waE2E.PollCreationMessage) string {
	if p.GetName() == "" {
		return "[Poll]"
	}

	return "[Poll: " + p.GetName() + "]"
}

// contactPlaceholder labels a shared contact card with its name, or
// just names it a contact when WhatsApp reported none.
func contactPlaceholder(name string) string {
	if name == "" {
		return "[Contact]"
	}

	return "[Contact: " + name + "]"
}

// audioPlaceholder distinguishes a voice note from any other audio
// message, since WhatsApp reports both as the same message kind.
func audioPlaceholder(a *waE2E.AudioMessage) string {
	if a.GetPTT() {
		return "[Voice message]"
	}

	return "[Audio]"
}
