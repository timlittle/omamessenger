package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// PollUpdated updates a stored message's poll after the service reports
// its options or tallies changing without a full edit, such as someone
// voting live. It never notifies; the open conversation refreshes the
// message in place. A sender fully controls a poll's own question,
// option count and option text the same way they control a message's
// own text, so poll gets the same cap domain.SanitizeIncoming gives
// one arriving as part of a message (see domain.CapPoll).
func (in *Ingest) PollUpdated(ctx context.Context, accountID, conversationRemoteID, messageRemoteID string, poll domain.Poll) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	updated, found, err := in.store.SetPoll(ctx, conv.ID, messageRemoteID, domain.CapPoll(poll))
	if err != nil || !found {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, updated)
}
