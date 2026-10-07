package whatsapp

// send.go delivers outgoing text messages: Connector.Send hands a
// message to whatsmeow and reports it sent, with WhatsApp's own id for
// it, through the sink the account's current run is using. Sending media
// is a later wave's job.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// sendTimeout bounds how long Send waits for WhatsApp to accept a
// message, so an account with network trouble cannot leave a caller
// waiting forever.
const sendTimeout = 30 * time.Second

// errMediaNotSupported reports an attachment Send cannot deliver yet.
var errMediaNotSupported = errors.New("whatsapp: sending attachments is not supported yet")

// Send delivers m's text to conv and reports it sent, with WhatsApp's id
// for it, before returning. Reporting it here rather than only through
// the later receipt lets a fast send already show as sent, not pending,
// by the time the caller re-reads the stored message.
func (c *Connector) Send(ctx context.Context, conv domain.Conversation, m domain.Message) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	if m.Media != nil {
		return errMediaNotSupported
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	id := dev.generateMessageID()
	if _, err := dev.sendMessage(sendCtx, jid, outgoingMessage(m), id); err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}

	c.trackSent(sentKey(conv.RemoteID, string(id)), m.ID, expectedRecipients(conv))
	sink.OutgoingStatus(ctx, m.ID, string(id), domain.StatusSent)

	return nil
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

// outgoingMessage builds the WhatsApp message proto for m: its text
// alone, or, when it replies to another message, an extended text
// message quoting that message's stanza id. The stored Reply keeps only
// the quoted message's remote id and a display name (see domain.Reply),
// not its sender's JID or its own WhatsApp content, so this cannot fill
// in ContextInfo's Participant or QuotedMessage. WhatsApp still renders
// the quote correctly from the id alone, since the recipient's own
// client already holds a copy of the quoted message.
func outgoingMessage(m domain.Message) *waE2E.Message {
	if m.ReplyTo == nil || m.ReplyTo.RemoteID == "" {
		return &waE2E.Message{Conversation: strp(m.Text)}
	}

	return &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        strp(m.Text),
			ContextInfo: &waE2E.ContextInfo{StanzaID: strp(m.ReplyTo.RemoteID)},
		},
	}
}

// strp takes the address of a string, for the generated protobuf structs
// that hold every optional field as a pointer.
func strp(s string) *string { return &s }
