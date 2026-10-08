package telegram

// vote.go implements connector.Voter: casting the signed-in user's
// choice in a poll with messages.sendVote.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Voter = (*Connector)(nil)

// Vote casts the signed-in user's choice in the poll messageRemoteID
// carries, replacing any previous vote Telegram already has for it,
// and reports the poll's resulting tally back through the sink at
// once, the way React echoes its own response rather than waiting for
// a matching live update.
func (c *Connector) Vote(ctx context.Context, conv domain.Conversation, messageRemoteID string, optionIDs []string) error {
	api, sink, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	msgID, err := strconv.Atoi(messageRemoteID)
	if err != nil {
		return fmt.Errorf("telegram: vote: %w", err)
	}

	options, err := pollOptionsBytes(optionIDs)
	if err != nil {
		return fmt.Errorf("telegram: vote: %w", err)
	}

	result, err := api.MessagesSendVote(ctx, &tg.MessagesSendVoteRequest{Peer: peer, MsgID: msgID, Options: options})
	if err != nil {
		return fmt.Errorf("telegram: vote: %w", err)
	}

	if updater, ok := sink.(connector.PollUpdater); ok {
		if poll, ok := pollFromUpdates(result); ok {
			updater.PollUpdated(ctx, c.account.ID, conv.RemoteID, messageRemoteID, poll)
		}
	}

	return nil
}

// pollOptionsBytes turns the chosen option ids back into the raw bytes
// messages.sendVote expects.
func pollOptionsBytes(optionIDs []string) ([][]byte, error) {
	options := make([][]byte, len(optionIDs))
	for i, id := range optionIDs {
		b, err := pollOptionBytes(id)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", id, err)
		}
		options[i] = b
	}

	return options, nil
}

// pollFromUpdates finds the poll update Telegram's own response to
// messages.sendVote carries, or false when it carries none, the way
// reactionsFromUpdates reads the same response for React.
func pollFromUpdates(result tg.UpdatesClass) (domain.Poll, bool) {
	updates, ok := result.(*tg.Updates)
	if !ok {
		return domain.Poll{}, false
	}

	for _, u := range updates.Updates {
		if up, ok := u.(*tg.UpdateMessagePoll); ok {
			return pollUpdate(up), true
		}
	}

	return domain.Poll{}, false
}
