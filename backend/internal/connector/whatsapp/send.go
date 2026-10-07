package whatsapp

// send.go delivers outgoing messages: Connector.Send hands a plain,
// quoted-reply or media message to whatsmeow and reports it sent, with
// WhatsApp's own id for it, through the sink the account's current run
// is using. Uploading an attachment itself is upload.go's job.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// sendTimeout bounds how long Send waits for WhatsApp to accept a
// message, so an account with network trouble cannot leave a caller
// waiting forever.
const sendTimeout = 30 * time.Second

// Send delivers m's text or attachment to conv and reports it sent,
// with WhatsApp's id for it, before returning. Reporting it here rather
// than only through the later receipt lets a fast send already show as
// sent, not pending, by the time the caller re-reads the stored
// message.
func (c *Connector) Send(ctx context.Context, conv domain.Conversation, m domain.Message) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	msg, err := buildOutgoing(sendCtx, dev, m)
	if err != nil {
		return err
	}

	id := dev.generateMessageID()
	if _, err := dev.sendMessage(sendCtx, jid, msg, id); err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}

	c.saveOutgoingRef(ctx, conv.RemoteID, string(id), msg)
	c.trackSent(sentKey(conv.RemoteID, string(id)), m.ID, expectedRecipients(conv))
	sink.OutgoingStatus(ctx, m.ID, string(id), domain.StatusSent)

	return nil
}

// buildOutgoing builds m's message proto: a plain or quoted-reply text
// message, or an uploaded attachment.
func buildOutgoing(ctx context.Context, dev device, m domain.Message) (*waE2E.Message, error) {
	if m.Media == nil {
		return outgoingTextMessage(m), nil
	}

	return uploadAttachment(ctx, dev, m)
}

// saveOutgoingRef remembers an outgoing attachment's reference under
// the wire id WhatsApp gave it, so FetchMedia can download this
// account's own sent copy back later; msg carries nothing to save when
// m had no attachment. Saving is best effort: a failed save only means
// this one message cannot be re-downloaded later, which matters far
// less than the message already being on its way.
func (c *Connector) saveOutgoingRef(ctx context.Context, conversationRemoteID, messageRemoteID string, msg *waE2E.Message) {
	ref, ok := downloadRef(msg)
	if !ok {
		return
	}

	media, err := c.currentMedia()
	if err != nil {
		return
	}

	_ = media.put(ctx, conversationRemoteID, messageRemoteID, ref) // best effort; see doc comment above
}

// expectedRecipients is how many other participants a group message
// must reach before its delivered or read tick advances, matching
// WhatsApp's own behaviour of showing a tick only once every member has
// caught up (see receipts.go); a direct chat always has exactly one.
// conv.Members, synced from WhatsApp's own chat list, is the closest
// count this connector has without an extra round trip per send; it
// floors at 1 so a group whose member count is not known yet still
// advances on its first receipt rather than never advancing at all.
func expectedRecipients(conv domain.Conversation) int {
	if conv.Kind != domain.KindGroup {
		return 1
	}
	if conv.Members > 1 {
		return conv.Members - 1
	}

	return 1
}

// outgoingTextMessage builds the WhatsApp message proto for a text-only
// m: its text alone, or, when it replies to another message, an
// extended text message quoting that message's stanza id.
func outgoingTextMessage(m domain.Message) *waE2E.Message {
	ctxInfo := replyContext(m.ReplyTo)
	if ctxInfo == nil {
		return &waE2E.Message{Conversation: strp(m.Text)}
	}

	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strp(m.Text), ContextInfo: ctxInfo}}
}

// replyContext is the quoting WhatsApp needs for a message that
// replies to another, by stanza id alone, or nil when it does not
// reply to anything. The stored Reply keeps only the quoted message's
// remote id and a display name (see domain.Reply), not its sender's
// JID or its own WhatsApp content, so this cannot fill in
// ContextInfo's Participant or QuotedMessage. WhatsApp still renders
// the quote correctly from the id alone, since the recipient's own
// client already holds a copy of the quoted message.
func replyContext(reply *domain.Reply) *waE2E.ContextInfo {
	if reply == nil || reply.RemoteID == "" {
		return nil
	}

	return &waE2E.ContextInfo{StanzaID: strp(reply.RemoteID)}
}

// wireCaption is m's text to send as its attachment's caption: empty
// when it is only the placeholder domain.MediaPlaceholder gives media
// sent with no caption, which must never reach the other side as real
// text.
func wireCaption(m domain.Message) string {
	if m.Media != nil && m.Text == domain.MediaPlaceholder(m.Media.Kind) {
		return ""
	}

	return m.Text
}

// strp takes the address of a string, for the generated protobuf structs
// that hold every optional field as a pointer.
func strp(s string) *string { return &s }
