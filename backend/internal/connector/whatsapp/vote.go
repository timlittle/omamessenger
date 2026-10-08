package whatsapp

// vote.go implements connector.Voter: casting the signed-in user's
// choice in a poll with whatsmeow's BuildPollVote, and decrypting a
// vote whatsmeow reports arriving, ours or anyone else's, with
// DecryptPollVote. WhatsApp identifies an option by the SHA-256 hash of
// its text rather than a message-specific id (see poll.go's
// pollOptionID), so a vote decrypts to hashes alone, with no text and
// no running tally of its own; pollstore.go is what turns that back
// into a tally against the right option.

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Voter = (*Connector)(nil)

// voteTimeout bounds how long Vote waits for WhatsApp to accept the
// vote, matching Send's and React's own timeouts.
const voteTimeout = 30 * time.Second

// errUnknownVoteTarget reports a vote asked for a poll this connector
// never recorded the sender of, so it cannot build the key WhatsApp
// needs to identify it.
var errUnknownVoteTarget = errors.New("whatsapp: vote: unknown poll")

// errUnknownPollOption reports a vote for an option id this connector
// never saved for the poll, so it has no option text to cast a vote
// with.
var errUnknownPollOption = errors.New("whatsapp: vote: unknown option")

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

	pollInfo, err := votePollInfo(ctx, media, conv.RemoteID, messageRemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: vote: %w", err)
	}

	names, err := optionNames(ctx, media, conv.RemoteID, messageRemoteID, optionIDs)
	if err != nil {
		return fmt.Errorf("whatsapp: vote: %w", err)
	}

	voteCtx, cancel := context.WithTimeout(ctx, voteTimeout)
	defer cancel()

	msg, err := dev.buildPollVote(voteCtx, pollInfo, names)
	if err != nil {
		return fmt.Errorf("whatsapp: vote: %w", err)
	}

	if _, err := dev.sendMessage(voteCtx, pollInfo.Chat, msg, dev.generateMessageID()); err != nil {
		return fmt.Errorf("whatsapp: vote: %w", err)
	}

	c.reportTally(ctx, media, sink, voteReport{conversationRemoteID: conv.RemoteID, messageRemoteID: messageRemoteID, voterID: "self", optionIDs: optionIDs})

	return nil
}

// votePollInfo rebuilds the message info BuildPollVote needs to
// identify the poll: the chat, the poll's own sender and message id,
// read back from the key saved when the poll creation message first
// arrived (see keys.go).
func votePollInfo(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string) (*types.MessageInfo, error) {
	chat, err := jidFromRemoteID(conversationRemoteID)
	if err != nil {
		return nil, err
	}

	key, found, err := media.messageKeyFor(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errUnknownVoteTarget
	}

	sender := chat
	if !key.fromMe {
		sender, err = jidFromRemoteID(key.senderID)
		if err != nil {
			return nil, err
		}
	}

	return &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat: chat, Sender: sender, IsFromMe: key.fromMe, IsGroup: chat.Server == types.GroupServer,
		},
		ID: types.MessageID(messageRemoteID),
	}, nil
}

// optionNames turns the chosen option ids back into the option text
// WhatsApp's vote protocol hashes, from the poll saved when its
// creation message first arrived (see pollstore.go).
func optionNames(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, optionIDs []string) ([]string, error) {
	shell, found, err := media.pollShell(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errUnknownVoteTarget
	}

	byID := make(map[string]string, len(shell.Options))
	for _, o := range shell.Options {
		byID[o.ID] = o.Text
	}

	names := make([]string, len(optionIDs))
	for i, id := range optionIDs {
		name, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errUnknownPollOption, id)
		}
		names[i] = name
	}

	return names, nil
}

// handlePollVote decrypts a live poll vote, folds it into that poll's
// tally and reports the result, dropping the update when it cannot be
// decrypted, its poll was never saved, or the sink given to this run
// does not keep polls (see connector.PollUpdater).
func (c *Connector) handlePollVote(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Message) {
	if media == nil {
		return
	}

	if _, ok := sink.(connector.PollUpdater); !ok {
		return
	}

	vote, err := dev.decryptPollVote(ctx, e)
	if err != nil {
		logPollVoteDecryptFailed()
		return
	}

	pollMessageID := e.Message.GetPollUpdateMessage().GetPollCreationMessageKey().GetID()
	optionIDs := make([]string, len(vote.GetSelectedOptions()))
	for i, h := range vote.GetSelectedOptions() {
		optionIDs[i] = hex.EncodeToString(h)
	}

	remote := chatID(ctx, dev, e.Info.Chat)
	c.reportTally(ctx, media, sink, voteReport{
		conversationRemoteID: remote, messageRemoteID: pollMessageID, voterID: reactorKey(e.Info), optionIDs: optionIDs,
	})
}

// voteReport bundles what reportTally needs to save a vote and report a
// poll's resulting tally, keeping that method's own parameter count
// within this codebase's limit.
type voteReport struct {
	conversationRemoteID string
	messageRemoteID      string
	voterID              string
	optionIDs            []string
}

// reportTally saves report's chosen options for a poll and reports its
// recomputed tally through the sink, if it keeps polls and the poll was
// saved in the first place.
func (c *Connector) reportTally(ctx context.Context, media *mediaStore, sink connector.Sink, report voteReport) {
	updater, ok := sink.(connector.PollUpdater)
	if !ok {
		return
	}

	if err := media.setVotes(ctx, report.conversationRemoteID, report.messageRemoteID, report.voterID, report.optionIDs); err != nil {
		return
	}

	poll, found, err := media.pollTally(ctx, report.conversationRemoteID, report.messageRemoteID, "self")
	if err != nil || !found {
		return
	}

	updater.PollUpdated(ctx, c.account.ID, report.conversationRemoteID, report.messageRemoteID, poll)
}
