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
// alone, or, when it replies to another message, @-mentions a group
// member, or sits in a chat with disappearing messages on, an extended
// text message carrying the ContextInfo any of those needs.
func outgoingMessage(ctx context.Context, target sendTarget, m domain.Message) *waE2E.Message {
	text, mentionedJIDs := outgoingMentions(m.Text, m.Mentions)
	expireAfter := uint32(m.ExpireAfter)
	if m.ReplyTo == nil || m.ReplyTo.RemoteID == "" {
		return plainOrMentionedMessage(text, mentionedJIDs, expireAfter)
	}

	info := quoteContext(ctx, target, m.ReplyTo, expireAfter)
	info.MentionedJID = mentionedJIDs

	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strp(text), ContextInfo: info}}
}

// plainOrMentionedMessage builds a text-only message proto: a plain
// conversation message, or, once text carries any @-mentions or the
// chat's own disappearing timer applies, an extended text message with
// the ContextInfo WhatsApp's clients need to render either. Shared by
// outgoingMessage's own no-reply case and by EditMessage (see edit.go),
// which always passes 0 for expireAfter: an edit never needs its own
// expiration, since the original message's already governs it.
func plainOrMentionedMessage(text string, mentionedJIDs []string, expireAfter uint32) *waE2E.Message {
	if len(mentionedJIDs) == 0 && expireAfter == 0 {
		return &waE2E.Message{Conversation: strp(text)}
	}

	info := &waE2E.ContextInfo{MentionedJID: mentionedJIDs}
	if expireAfter > 0 {
		info.Expiration = &expireAfter
	}

	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strp(text), ContextInfo: info}}
}

// quoteContext builds the ContextInfo that quotes reply and carries
// expireAfter, the chat's own disappearing timer in seconds (0 for
// none): the stanza id always, and, when message_keys has a record of
// who sent it, the participant to credit it to in a group and its
// text, so the recipient's client can render the quote even before it
// has the original cached.
func quoteContext(ctx context.Context, target sendTarget, reply *domain.Reply, expireAfter uint32) *waE2E.ContextInfo {
	info := &waE2E.ContextInfo{StanzaID: strp(reply.RemoteID)}
	if expireAfter > 0 {
		info.Expiration = &expireAfter
	}
	if target.media == nil {
		return info
	}

	key, found, err := target.media.messageKeyFor(ctx, target.conversationRemoteID, reply.RemoteID)
	if err != nil || !found {
		return info
	}

	if !key.fromMe && target.chat.Server == types.GroupServer && key.senderID != "" {
		info.Participant = strp(key.senderID)
	}
	if reply.Text != "" {
		info.QuotedMessage = &waE2E.Message{Conversation: strp(reply.Text)}
	}

	return info
}
