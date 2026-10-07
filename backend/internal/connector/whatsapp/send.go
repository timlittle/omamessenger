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

	media := c.mediaFor()
	id := dev.generateMessageID()
	msg := outgoingMessage(sendCtx, media, conv.RemoteID, jid, m)
	if _, err := dev.sendMessage(sendCtx, jid, msg, id); err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}

	saveMessageKey(sendCtx, media, conv.RemoteID, string(id), messageKey{fromMe: true})
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

// strp takes the address of a string, for the generated protobuf structs
// that hold every optional field as a pointer.
func strp(s string) *string { return &s }
