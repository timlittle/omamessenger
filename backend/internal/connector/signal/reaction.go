package signal

import (
	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// maxTrackedReactions bounds how many messages' own reaction chips one
// run remembers who reacted to, the same way the WhatsApp connector
// bounds its own in-memory reaction table.
const maxTrackedReactions = 1024

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
