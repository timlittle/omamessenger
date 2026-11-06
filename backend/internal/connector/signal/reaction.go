package signal

// reaction.go also implements connector.Reactor: adding or clearing
// this account's own reaction to a message by sending Signal's own
// Reaction field on a DataMessage. Signal requires an emoji even to
// clear a reaction (Remove alone is not enough), so clearing reuses
// whichever emoji this run last saw self react with; a run that never
// saw it - a reaction set before this run started, or on another
// device - falls back to a generic emoji rather than failing outright.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Reactor = (*Connector)(nil)

// maxTrackedReactions bounds how many messages' own reaction chips one
// run remembers who reacted to, the same way the WhatsApp connector
// bounds its own in-memory reaction table.
const maxTrackedReactions = 1024

// reactTimeout bounds how long React waits for Signal to accept the
// reaction, matching Send's own send timeout.
const reactTimeout = 30 * time.Second

// fallbackClearEmoji is sent with Remove when no earlier emoji from
// self is on record for a message, since Signal's own Reaction field
// always needs one.
const fallbackClearEmoji = "👍"

// React sets or clears this account's own reaction to a message and
// reports the message's resulting tally through the sink.
func (c *Connector) React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	targetAuthor, targetTimestamp, err := parseMessageRemoteID(messageRemoteID)
	if err != nil {
		return fmt.Errorf("signal: react: %w", err)
	}

	self := c.selfID()
	key := conv.RemoteID + "/" + messageRemoteID
	remove := emoji == ""
	sendEmoji := emoji
	if remove {
		sendEmoji = c.selfEmoji(key, self, fallbackClearEmoji)
	}

	reactCtx, cancel := context.WithTimeout(ctx, reactTimeout)
	defer cancel()

	dm := &signalpb.DataMessage{
		Timestamp: u64p(uint64(time.Now().UnixMilli())),
		Reaction: &signalpb.DataMessage_Reaction{
			Emoji: strp(sendEmoji), Remove: boolp(remove),
			TargetAuthorAci: strp(senderRemoteID(targetAuthor)), TargetSentTimestamp: u64p(targetTimestamp),
		},
	}
	if err := sendToChat(reactCtx, dev, conv.RemoteID, signalmeow.WrapDataMessage(dm)); err != nil {
		return fmt.Errorf("signal: react: %w", err)
	}

	chips := c.setReaction(key, self, emoji, self)
	sink.Reacted(ctx, c.account.ID, conv.RemoteID, messageRemoteID, chips)

	return nil
}

// selfEmoji returns self's current reaction to the message key names,
// or fallback when this run has no reaction recorded for it.
func (c *Connector) selfEmoji(key string, self uuid.UUID, fallback string) string {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if emoji := c.reactions[key][self]; emoji != "" {
		return emoji
	}

	return fallback
}

// setReaction records who reacted to messageKey (a conversation and
// message remote id pair) with emoji, or clears their reaction when
// emoji is "", and reports the resulting chips.
func (c *Connector) setReaction(messageKey string, sender uuid.UUID, emoji string, self uuid.UUID) []domain.Reaction {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if c.reactions == nil {
		c.reactions = map[string]map[uuid.UUID]string{}
	}
	if _, exists := c.reactions[messageKey]; !exists && len(c.reactions) >= maxTrackedReactions {
		oldest, rest := c.reactionOrder[0], c.reactionOrder[1:]
		delete(c.reactions, oldest)
		c.reactionOrder = rest
	}
	if c.reactions[messageKey] == nil {
		c.reactions[messageKey] = map[uuid.UUID]string{}
		c.reactionOrder = append(c.reactionOrder, messageKey)
	}

	if emoji == "" {
		delete(c.reactions[messageKey], sender)
	} else {
		c.reactions[messageKey][sender] = emoji
	}

	return reactionChips(c.reactions[messageKey], self)
}

// reactionChips tallies by, grouping every reactor by emoji.
func reactionChips(by map[uuid.UUID]string, self uuid.UUID) []domain.Reaction {
	counts := map[string]int{}
	mine := map[string]bool{}
	var order []string

	for reactor, emoji := range by {
		if counts[emoji] == 0 {
			order = append(order, emoji)
		}
		counts[emoji]++
		if reactor == self {
			mine[emoji] = true
		}
	}

	chips := make([]domain.Reaction, 0, len(order))
	for _, emoji := range order {
		chips = append(chips, domain.Reaction{Emoji: emoji, Count: counts[emoji], Mine: mine[emoji]})
	}

	return chips
}
