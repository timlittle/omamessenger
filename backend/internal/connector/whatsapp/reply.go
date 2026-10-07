package whatsapp

// reply.go builds the outgoing message proto for Send: plain text, or,
// when it replies to another message, an extended text message with a
// ContextInfo WhatsApp clients use to render a proper quote above it.
// Building more than the bare stanza id needs message_keys's record of
// who sent the quoted message (see keys.go); when nothing was recorded
// for it, such as a message from before this connector tracked keys,
// only the stanza id is sent, and WhatsApp still threads the reply
// correctly, since the recipient's own client already holds a copy of
// the message being quoted.

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// outgoingMessage builds the WhatsApp message proto for m: its text
// alone, or, when it replies to another message, an extended text
// message quoting that message.
func outgoingMessage(ctx context.Context, media *mediaStore, conversationRemoteID string, chat types.JID, m domain.Message) *waE2E.Message {
	if m.ReplyTo == nil || m.ReplyTo.RemoteID == "" {
		return &waE2E.Message{Conversation: strp(m.Text)}
	}

	return &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        strp(m.Text),
			ContextInfo: quoteContext(ctx, media, conversationRemoteID, chat, m.ReplyTo),
		},
	}
}

// quoteContext builds the ContextInfo that quotes reply: its stanza id
// always, and, when message_keys has a record of who sent it, the
// participant to credit it to in a group and its text, so the
// recipient's client can render the quote even before it has the
// original cached.
func quoteContext(ctx context.Context, media *mediaStore, conversationRemoteID string, chat types.JID, reply *domain.Reply) *waE2E.ContextInfo {
	info := &waE2E.ContextInfo{StanzaID: strp(reply.RemoteID)}
	if media == nil {
		return info
	}

	key, found, err := media.messageKeyFor(ctx, conversationRemoteID, reply.RemoteID)
	if err != nil || !found {
		return info
	}

	if !key.fromMe && chat.Server == types.GroupServer && key.senderID != "" {
		info.Participant = strp(key.senderID)
	}
	if reply.Text != "" {
		info.QuotedMessage = &waE2E.Message{Conversation: strp(reply.Text)}
	}

	return info
}
