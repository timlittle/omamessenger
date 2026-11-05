package signal

import (
	"context"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// newPoll is a poll as created, before anyone votes.
type newPoll struct {
	question       string
	options        []string
	multipleChoice bool
}

// trackPoll saves a newly created poll's options under
// messageRemoteID, so a later vote or terminate can be folded in.
func (c *Connector) trackPoll(ctx context.Context, conversationRemoteID, messageRemoteID string, p newPoll) {
	media := c.mediaFor()
	if media == nil {
		return
	}

	_ = media.putPoll(ctx, conversationRemoteID, messageRemoteID, p)
}

// recordVote replaces voter's choice for the poll tracked under
// messageRemoteID and reports its current tallies, or found false.
func (c *Connector) recordVote(ctx context.Context, conversationRemoteID, messageRemoteID string, voter uuid.UUID, optionIndexes []int) (domain.Poll, bool) {
	media := c.mediaFor()
	if media == nil {
		return domain.Poll{}, false
	}

	if err := media.setVotes(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(voter), optionIndexes); err != nil {
		return domain.Poll{}, false
	}

	poll, found, err := media.pollTally(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(c.selfID()))
	if err != nil {
		return domain.Poll{}, false
	}

	return poll, found
}

// closePoll marks the poll tracked under messageRemoteID closed and
// reports its final tallies, or found false when this run never saw
// that poll created.
func (c *Connector) closePoll(ctx context.Context, conversationRemoteID, messageRemoteID string) (domain.Poll, bool) {
	media := c.mediaFor()
	if media == nil {
		return domain.Poll{}, false
	}

	if err := media.closePoll(ctx, conversationRemoteID, messageRemoteID); err != nil {
		return domain.Poll{}, false
	}

	poll, found, err := media.pollTally(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(c.selfID()))
	if err != nil {
		return domain.Poll{}, false
	}

	return poll, found
}
