package telegram

// edit.go implements connector.MessageEditor: changing the text of a
// message this account already sent, keeping its mentions as entities
// the same way an outgoing send does (see upload.go's sendRequest).

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.MessageEditor = (*Connector)(nil)

// EditMessage changes the text of the message remoteID names in conv to
// newText, with mentions reapplied as entities.
func (c *Connector) EditMessage(ctx context.Context, conv domain.Conversation, remoteID, newText string, mentions []domain.Mention) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(remoteID)
	if err != nil {
		return fmt.Errorf("telegram: edit: %w", err)
	}

	_, err = api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer: peer, ID: id, Message: newText, Entities: outgoingEntities(mentions),
	})
	if err != nil {
		return editError(err)
	}

	return nil
}

// editError wraps an edit failure Telegram reports for a reason the
// user can act on, such as its own time limit on editing a message or
// missing admin rights, with ErrEditUnsupported; anything else, such as
// a dropped connection, is returned wrapped but otherwise unchanged.
func editError(err error) error {
	if tgerr.Is(err, "MESSAGE_EDIT_TIME_EXPIRED", "MESSAGE_AUTHOR_REQUIRED", "CHAT_ADMIN_REQUIRED") {
		return fmt.Errorf("telegram: edit: %w", connector.ErrEditUnsupported)
	}

	return fmt.Errorf("telegram: edit: %w", err)
}
