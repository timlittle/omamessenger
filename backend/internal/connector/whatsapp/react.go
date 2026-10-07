package whatsapp

// react.go implements connector.Reactor: adding or clearing this
// account's own reaction to a message. WhatsApp represents clearing a
// reaction as sending a new one with an empty emoji, so one method
// covers both; whichever it is, the message's resulting tally is folded
// into the same per-message cache a live reaction updates (see live.go)
// and reported through Sink.Reacted the same way, so the UI never has
// to tell a local reaction from a remote one.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Reactor = (*Connector)(nil)

// reactTimeout bounds how long React waits for WhatsApp to accept the
// reaction, matching Send's own send timeout.
const reactTimeout = 30 * time.Second

// errUnknownReactionTarget reports a reaction asked for a message this
// connector never recorded the sender of, so it cannot build the key
// WhatsApp needs to identify it, in particular which group member to
// credit it to.
var errUnknownReactionTarget = errors.New("whatsapp: react: unknown message")

// React sets or clears this account's own reaction to a message and
// reports the message's resulting tally through the sink.
func (c *Connector) React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil {
		return errNotConnected
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: react: %w", err)
	}

	key, found, err := media.messageKeyFor(ctx, conv.RemoteID, messageRemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: react: %w", err)
	}
	if !found {
		return errUnknownReactionTarget
	}

	reactCtx, cancel := context.WithTimeout(ctx, reactTimeout)
	defer cancel()

	msg := reactionMessage(chat, key, messageRemoteID, emoji)
	if _, err := dev.sendMessage(reactCtx, chat, msg, dev.generateMessageID()); err != nil {
		return fmt.Errorf("whatsapp: react: %w", err)
	}

	tally := c.reactTo(conv.RemoteID, messageRemoteID, "self", emoji)
	sink.Reacted(ctx, c.account.ID, conv.RemoteID, messageRemoteID, tally)

	return nil
}

// reactionMessage builds the WhatsApp message that sets, or with ""
// clears, this account's own reaction to the message key identifies.
func reactionMessage(chat types.JID, key messageKey, messageRemoteID, emoji string) *waE2E.Message {
	return &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Key:               targetKey(chat, key, messageRemoteID),
		Text:              strp(emoji),
		SenderTimestampMS: int64p(time.Now().UnixMilli()),
	}}
}

// int64p takes the address of an int64, for the generated protobuf
// structs that hold every optional field as a pointer.
func int64p(n int64) *int64 { return &n }
