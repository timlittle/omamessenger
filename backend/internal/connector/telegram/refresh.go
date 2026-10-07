package telegram

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// refreshBatch is the most message ids Telegram accepts in one
// messages.getMessages or channels.getMessages request.
const refreshBatch = 100

// RefreshMessages re-reports the messages Telegram knows by remoteIDs,
// through the Sink's History, so the store can fill in media or a link
// preview a message had none of when it was first synced.
func (c *Connector) RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error {
	api, sink, err := c.session()
	if err != nil {
		return err
	}

	ids, err := parseRemoteIDs(remoteIDs)
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	r := messageRefresh{api: api, sink: sink, conv: conv, peer: peer}
	for chunk := range slices.Chunk(ids, refreshBatch) {
		if err := r.chunk(ctx, chunk); err != nil {
			return err
		}
	}

	return nil
}

// messageRefresh fetches batches of one conversation's messages by id and
// reports each through the Sink's History.
type messageRefresh struct {
	api  *tg.Client
	sink connector.Sink
	conv domain.Conversation
	peer tg.InputPeerClass
}

// chunk fetches one batch of messages by id and reports each.
func (r messageRefresh) chunk(ctx context.Context, ids []int) error {
	want := make([]tg.InputMessageClass, len(ids))
	for i, id := range ids {
		want[i] = &tg.InputMessageID{ID: id}
	}

	result, err := fetchMessages(ctx, r.api, r.peer, want)
	if err != nil {
		return fmt.Errorf("telegram: refresh messages: %w", err)
	}

	messages, ok := result.AsModified()
	if !ok {
		return nil
	}

	e := newEntities(messages.GetUsers(), messages.GetChats())
	for _, m := range messages.GetMessages() {
		if msg, ok := m.(*tg.Message); ok {
			r.sink.History(ctx, r.conv.AccountID, r.conv.RemoteID, message(msg, e))
		}
	}

	return nil
}

// parseRemoteIDs converts Telegram's string message ids back to the ints
// its API takes.
func parseRemoteIDs(remoteIDs []string) ([]int, error) {
	ids := make([]int, len(remoteIDs))
	for i, s := range remoteIDs {
		id, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("telegram: refresh messages: %w", err)
		}
		ids[i] = id
	}

	return ids, nil
}
