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
	"slices"
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

// maxReactionTargets bounds how many distinct messages this connector
// remembers a live reaction tally for at once. WhatsApp reports one
// person's reaction change at a time, never a conversation's own full
// tally (see reactionTally in normalize_events.go), so this connector
// must keep its own running tally for as long as a message might still
// change again; without a bound, a sender naming an endless stream of
// message ids - real or entirely fabricated - in their own
// ReactionMessage.Key would grow c.reactions forever. Generous for any
// real account's own messages actively collecting reactions at once;
// past it, the oldest message tracked so far is evicted to make room.
const maxReactionTargets = 2000

// maxReactorsPerMessage bounds how many distinct senders' reactions one
// message's own tally remembers, the same cap a group's own member list
// is already bounded to (domain.MaxGroupMembers): nobody reacting to one
// message can ever outnumber that group's own membership. A reactor
// past this cap is refused rather than displacing one already on
// record, so an existing reactor's own chip is never wrongly zeroed to
// make room for a new one.
const maxReactorsPerMessage = domain.MaxGroupMembers

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

// reactTo records sender's reaction to a message as emoji, or clears it
// when emoji is "", and returns the message's full, recomputed tally. A
// sender fully controls both the emoji text and which message id their
// own reaction names, so this bounds every part of what it keeps: an
// oversized emoji (domain.MaxReactionEmojiLength) is treated the same
// as an empty one, a brand new message tracked past
// maxReactionTargets evicts the oldest message tracked so far, and a
// brand new reactor past maxReactorsPerMessage on one message is
// refused rather than displacing an existing vote. A message whose
// tally drops back to empty is forgotten outright, so a sender cannot
// pad c.reactions by reacting then immediately clearing.
func (c *Connector) reactTo(conversationRemoteID, messageRemoteID, sender, emoji string) []domain.Reaction {
	c.mu.Lock()
	defer c.mu.Unlock()

	if domain.UTF16Len(emoji) > domain.MaxReactionEmojiLength {
		emoji = ""
	}

	if c.reactions == nil {
		c.reactions = map[string]map[string]string{}
	}

	key := conversationRemoteID + "/" + messageRemoteID
	bySender, tracked := c.reactions[key]
	if !tracked {
		bySender = map[string]string{}
		c.trackReactionTarget(key, bySender)
	}

	switch {
	case emoji == "":
		delete(bySender, sender)
	case len(bySender) < maxReactorsPerMessage || bySender[sender] != "":
		bySender[sender] = emoji
	}

	if len(bySender) == 0 {
		delete(c.reactions, key)
		c.forgetReactionTarget(key)
	}

	return reactionTally(bySender, "self")
}

// trackReactionTarget starts remembering key's own tally as bySender,
// evicting the oldest tracked message first if maxReactionTargets is
// already reached. Called with c.mu already held.
func (c *Connector) trackReactionTarget(key string, bySender map[string]string) {
	if len(c.reactionOrder) >= maxReactionTargets {
		oldest := c.reactionOrder[0]
		c.reactionOrder = c.reactionOrder[1:]
		delete(c.reactions, oldest)
	}

	c.reactions[key] = bySender
	c.reactionOrder = append(c.reactionOrder, key)
}

// forgetReactionTarget drops key from c.reactionOrder once its own
// tally has been removed from c.reactions. Called with c.mu already
// held.
func (c *Connector) forgetReactionTarget(key string) {
	if i := slices.Index(c.reactionOrder, key); i >= 0 {
		c.reactionOrder = slices.Delete(c.reactionOrder, i, i+1)
	}
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
