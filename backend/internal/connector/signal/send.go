package signal

// send.go delivers outgoing messages: Connector.Send builds a
// DataMessage from m - its text, formatting, mentions, reply quote and
// disappearing timer, plus an uploaded attachment when it carries one
// - and hands it to signalmeow through SendMessage or SendGroupMessage,
// whichever conv.RemoteID names. Uploading an attachment itself is
// upload.go's job; quoting a reply is reply.go's.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// sendTimeout bounds how long Send waits for Signal to accept a
// message, so an account with network trouble cannot leave a caller
// waiting forever.
const sendTimeout = 30 * time.Second

// Send delivers m's text or attachment to conv and reports it sent,
// with this account's own id for it, before returning.
func (c *Connector) Send(ctx context.Context, conv domain.Conversation, m domain.Message) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	dm, err := outgoingDataMessage(sendCtx, dev, m)
	if err != nil {
		return classifySendErr(err)
	}

	if err := sendToChat(sendCtx, dev, conv.RemoteID, signalmeow.WrapDataMessage(dm)); err != nil {
		return classifySendErr(err)
	}

	remoteID := messageRemoteID(c.selfID(), dm.GetTimestamp())
	if len(dm.GetAttachments()) > 0 {
		c.rememberAttachment(sendCtx, conv.RemoteID, remoteID, dm.GetAttachments()[0])
	}
	sink.OutgoingStatus(ctx, m.ID, remoteID, domain.StatusSent)

	return nil
}

// outgoingDataMessage builds m's DataMessage: its text, formatting,
// mentions, reply quote, disappearing timer and, when it carries one,
// its uploaded attachment.
func outgoingDataMessage(ctx context.Context, dev device, m domain.Message) (*signalpb.DataMessage, error) {
	dm := &signalpb.DataMessage{
		Body:       strp(wireCaption(m)),
		Timestamp:  u64p(uint64(time.Now().UnixMilli())),
		BodyRanges: outgoingBodyRanges(m.Formatting, m.Mentions),
	}

	if m.ExpireAfter > 0 {
		dm.ExpireTimer = u32p(uint32(m.ExpireAfter))
	}
	if m.ReplyTo != nil && m.ReplyTo.RemoteID != "" {
		dm.Quote = outgoingQuote(m.ReplyTo)
	}

	if m.Media != nil {
		ptr, err := uploadAttachment(ctx, dev, m)
		if err != nil {
			return nil, err
		}

		dm.Attachments = []*signalpb.AttachmentPointer{ptr}
	}

	return dm, nil
}

// sendToChat sends content to a group's current members or to a
// direct chat's own account, whichever conversationRemoteID names.
func sendToChat(ctx context.Context, dev device, conversationRemoteID string, content *signalpb.Content) error {
	if isGroupChat(conversationRemoteID) {
		return dev.sendGroup(ctx, types.GroupIdentifier(conversationRemoteID), content)
	}

	aci, err := parseACI(conversationRemoteID)
	if err != nil {
		return fmt.Errorf("signal: send: %w", err)
	}

	return dev.sendDirect(ctx, aci, content)
}

// classifySendErr wraps err so the app layer's retry scheduler can
// tell a permanent refusal - the recipient is not a Signal user at all
// - from one worth retrying.
func classifySendErr(err error) error {
	if errors.Is(err, signalmeow.ErrUnregisteredUser) {
		return fmt.Errorf("signal: send: %w: %w", connector.ErrSendPermanent, err)
	}

	return fmt.Errorf("signal: send: %w", err)
}

// wireCaption is m's text to send: empty when it is only the
// placeholder domain.MediaPlaceholder gives media sent with no
// caption, which must never reach the other side as real text.
func wireCaption(m domain.Message) string {
	if m.Media != nil && m.Text == domain.MediaPlaceholder(m.Media.Kind) {
		return ""
	}

	return m.Text
}
