package signal

import (
	"context"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// trackPoll saves a newly created poll's options under
// messageRemoteID, so a later vote or terminate can be folded in.
func (c *Connector) trackPoll(ctx context.Context, conversationRemoteID, messageRemoteID, question string, options []string, multipleChoice bool) {
	media := c.mediaFor()
	if media == nil {
		return
	}

	_ = media.putPoll(ctx, conversationRemoteID, messageRemoteID, question, options, multipleChoice)
}

// recordVote replaces voter's choice for the poll tracked under
// messageRemoteID and reports its current tallies, or found false.
func (c *Connector) recordVote(ctx context.Context, conversationRemoteID, messageRemoteID string, voter uuid.UUID, optionIndexes []int, self uuid.UUID) (domain.Poll, bool) {
	media := c.mediaFor()
	if media == nil {
		return domain.Poll{}, false
	}

	if err := media.setVotes(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(voter), optionIndexes); err != nil {
		return domain.Poll{}, false
	}

	poll, found, err := media.pollTally(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(self))
	if err != nil {
		return domain.Poll{}, false
	}

	return poll, found
}

// closePoll marks the poll tracked under messageRemoteID closed and
// reports its final tallies, or found false when this run never saw
// that poll created.
func (c *Connector) closePoll(ctx context.Context, conversationRemoteID, messageRemoteID string, self uuid.UUID) (domain.Poll, bool) {
	media := c.mediaFor()
	if media == nil {
		return domain.Poll{}, false
	}

	if err := media.closePoll(ctx, conversationRemoteID, messageRemoteID); err != nil {
		return domain.Poll{}, false
	}

	poll, found, err := media.pollTally(ctx, conversationRemoteID, messageRemoteID, senderRemoteID(self))
	if err != nil {
		return domain.Poll{}, false
	}

	return poll, found
}
