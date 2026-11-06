package signal

// vote.go implements connector.Voter: casting the signed-in user's
// choice in a poll by sending Signal's own DataMessage.PollVote,
// identifying each option by its index rather than any id of its own
// (see poll.go's formatOptionID). Signal's own vote protocol needs a
// per-voter counter that increases with every vote this account casts
// in the same poll, to tell a resent or reordered vote apart from a
// genuinely new one; signalmeow keeps no such counter, so this
// connector persists one of its own (see storage.go's
// nextVoteCount), the same way it persists everything else
// signalmeow itself does not.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Voter = (*Connector)(nil)

// voteTimeout bounds how long Vote waits for Signal to accept the
// vote, matching Send's and React's own timeouts.
const voteTimeout = 30 * time.Second

// errUnknownPollOption reports a vote for an option id that is not one
// formatOptionID ever produced, so it cannot be turned back into the
// option index Signal's own vote protocol wants.
var errUnknownPollOption = errors.New("signal: vote: unknown option")

// Vote casts the signed-in user's choice in a poll and reports the
// poll's resulting tally through the sink at once, the way React
// echoes its own response rather than waiting for a matching live
// update.
func (c *Connector) Vote(ctx context.Context, conv domain.Conversation, messageRemoteID string, optionIDs []string) error {
	dev, sink, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil {
		return errNotConnected
	}

	targetAuthor, targetTimestamp, err := parseMessageRemoteID(messageRemoteID)
	if err != nil {
		return fmt.Errorf("signal: vote: %w", err)
	}

	indexes, err := optionIndexes(optionIDs)
	if err != nil {
		return fmt.Errorf("signal: vote: %w", err)
	}

	self := c.selfID()
	count, err := media.nextVoteCount(ctx, conv.RemoteID, messageRemoteID, senderRemoteID(self))
	if err != nil {
		return fmt.Errorf("signal: vote: %w", err)
	}

	voteCtx, cancel := context.WithTimeout(ctx, voteTimeout)
	defer cancel()

	dm := &signalpb.DataMessage{
		Timestamp: u64p(uint64(time.Now().UnixMilli())),
		PollVote: &signalpb.DataMessage_PollVote{
			TargetAuthorAciBinary: targetAuthor[:], TargetSentTimestamp: u64p(targetTimestamp),
			OptionIndexes: indexes, VoteCount: u32p(count),
		},
	}
	if err := sendToChat(voteCtx, dev, conv.RemoteID, signalmeow.WrapDataMessage(dm)); err != nil {
		return fmt.Errorf("signal: vote: %w", err)
	}

	poll, found := c.recordVote(ctx, conv.RemoteID, messageRemoteID, self, intIndexes(indexes))
	if found {
		c.reportPollUpdate(ctx, sink, conv.RemoteID, messageRemoteID, poll)
	}

	return nil
}

// optionIndexes turns chosen option ids back into the zero-based
// indexes Signal's own vote protocol wants.
func optionIndexes(optionIDs []string) ([]uint32, error) {
	out := make([]uint32, len(optionIDs))
	for i, id := range optionIDs {
		if len(id) != 1 || id[0] < 'a' {
			return nil, fmt.Errorf("%w: %q", errUnknownPollOption, id)
		}
		out[i] = uint32(id[0] - 'a')
	}

	return out, nil
}

// intIndexes converts Signal's own uint32 option indexes to the plain
// int slice recordVote's shared tally bookkeeping wants.
func intIndexes(indexes []uint32) []int {
	out := make([]int, len(indexes))
	for i, idx := range indexes {
		out[i] = int(idx)
	}

	return out
}
