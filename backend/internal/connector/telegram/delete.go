package telegram

// delete.go implements connector.Deleter: deleting messages from a
// Telegram conversation. A direct chat or basic group can delete only
// for this account or, with revoke, for everyone; a channel or
// supergroup's own delete call has no such choice, since Telegram
// always deletes those for everyone, so "for me" there is refused
// rather than silently doing the opposite of what was asked.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Deleter = (*Connector)(nil)

// DeleteMessages deletes remoteIDs from conv, for everyone when
// forEveryone is true.
func (c *Connector) DeleteMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	ids, err := messageIDs(remoteIDs)
	if err != nil {
		return err
	}

	if channel, ok := peer.(*tg.InputPeerChannel); ok {
		return deleteChannelMessages(ctx, api, channel, ids, forEveryone)
	}

	_, err = api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{Revoke: forEveryone, ID: ids})
	if err != nil {
		return deleteError(err)
	}

	return nil
}

// deleteChannelMessages deletes ids from a channel or supergroup, which
// Telegram always deletes for everyone; "for me" is refused rather than
// deleting for everyone when that was not asked for.
func deleteChannelMessages(ctx context.Context, api *tg.Client, channel *tg.InputPeerChannel, ids []int, forEveryone bool) error {
	if !forEveryone {
		return fmt.Errorf("telegram: delete: %w: a channel only ever deletes for everyone", connector.ErrDeleteUnsupported)
	}

	input := &tg.InputChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash}
	if _, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{Channel: input, ID: ids}); err != nil {
		return deleteError(err)
	}

	return nil
}

// messageIDs parses remoteIDs, each a Telegram message id, as the
// delete requests need them.
func messageIDs(remoteIDs []string) ([]int, error) {
	ids := make([]int, 0, len(remoteIDs))
	for _, remote := range remoteIDs {
		id, err := strconv.Atoi(remote)
		if err != nil {
			return nil, fmt.Errorf("telegram: delete: %w", err)
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// deleteError wraps a delete failure Telegram reports for a reason the
// user can act on, such as a time limit on revoking a message or
// missing admin rights, with ErrDeleteUnsupported; anything else, such
// as a dropped connection, is returned wrapped but otherwise unchanged.
func deleteError(err error) error {
	if tgerr.Is(err, "MESSAGE_DELETE_FORBIDDEN", "CHAT_ADMIN_REQUIRED") {
		return fmt.Errorf("telegram: delete: %w", connector.ErrDeleteUnsupported)
	}

	return fmt.Errorf("telegram: delete: %w", err)
}
