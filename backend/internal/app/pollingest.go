package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// PollUpdated updates a stored message's poll after the service reports
// its options or tallies changing without a full edit, such as someone
// voting live. It never notifies; the open conversation refreshes the
// message in place.
func (in *Ingest) PollUpdated(ctx context.Context, accountID, conversationRemoteID, messageRemoteID string, poll domain.Poll) {
	conv, err := in.store.ConversationByRemote(ctx, accountID, conversationRemoteID)
	if err != nil {
		return
	}

	updated, found, err := in.store.SetPoll(ctx, conv.ID, messageRemoteID, poll)
	if err != nil || !found {
		return
	}

	in.events.publish(ctx, EventMessageUpdated, updated)
}
