package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// handleUpdates reports live updates: new, edited and deleted messages,
// read receipts for what we sent, what the user read elsewhere, and
// typing.
func (c *Connector) handleUpdates(d tg.UpdateDispatcher, sink connector.Sink) {
	c.handleMessageUpdates(d, sink)
	c.handleReceiptUpdates(d, sink)
}

// handleMessageUpdates reports messages arriving, edited or deleted.
func (c *Connector) handleMessageUpdates(d tg.UpdateDispatcher, sink connector.Sink) {
	d.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		c.newMessage(ctx, sink, u.Message, e)
		return nil
	})

	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		c.newMessage(ctx, sink, u.Message, e)
		return nil
	})

	d.OnEditMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateEditMessage) error {
		c.editMessage(ctx, sink, u.Message, e)
		return nil
	})

	d.OnEditChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateEditChannelMessage) error {
		c.editMessage(ctx, sink, u.Message, e)
		return nil
	})

	d.OnDeleteMessages(func(ctx context.Context, _ tg.Entities, u *tg.UpdateDeleteMessages) error {
		// Users and basic groups share one message id space per account,
		// so this update carries no peer: the sink must search for them.
		c.deleteMessages(ctx, sink, "", u.Messages)
		return nil
	})

	d.OnDeleteChannelMessages(func(ctx context.Context, _ tg.Entities, u *tg.UpdateDeleteChannelMessages) error {
		remote, ok := c.lookup("channel:" + strconv.FormatInt(u.ChannelID, 10))
		if !ok {
			return nil
		}
		c.deleteMessages(ctx, sink, remote, u.Messages)
		return nil
	})
}

// handleReceiptUpdates reports read receipts, typing and unread counts.
func (c *Connector) handleReceiptUpdates(d tg.UpdateDispatcher, sink connector.Sink) {
	d.OnReadHistoryOutbox(func(ctx context.Context, _ tg.Entities, u *tg.UpdateReadHistoryOutbox) error {
		c.readUpTo(ctx, sink, shortKey(u.Peer), u.MaxID)
		return nil
	})

	d.OnUserTyping(func(ctx context.Context, _ tg.Entities, u *tg.UpdateUserTyping) error {
		c.typing(ctx, sink, u)
		return nil
	})

	d.OnReadHistoryInbox(func(ctx context.Context, _ tg.Entities, u *tg.UpdateReadHistoryInbox) error {
		c.unread(ctx, sink, shortKey(u.Peer), u.StillUnreadCount)
		return nil
	})

	d.OnReadChannelInbox(func(ctx context.Context, _ tg.Entities, u *tg.UpdateReadChannelInbox) error {
		c.unread(ctx, sink, "channel:"+strconv.FormatInt(u.ChannelID, 10), u.StillUnreadCount)
		return nil
	})
}

// editMessage reports a message changed after it was sent, dropping the
// update when its conversation is not known to us.
func (c *Connector) editMessage(ctx context.Context, sink connector.Sink, m tg.MessageClass, te tg.Entities) {
	msg, ok := m.(*tg.Message)
	if !ok {
		return
	}

	e := fromUpdate(te)
	conv, ok := peerConversation(c.account.ID, msg.PeerID, e)
	if !ok {
		remote, known := c.lookup(shortKey(msg.PeerID))
		if !known {
			return
		}
		conv.RemoteID = remote
	}

	sink.Edited(ctx, c.account.ID, conv.RemoteID, message(msg, e))
}

// deleteMessages reports messages removed from the service, named by
// Telegram's own ids for them. remote is the conversation they belonged
// to, or "" when the update named none.
func (c *Connector) deleteMessages(ctx context.Context, sink connector.Sink, remote string, messageIDs []int) {
	if len(messageIDs) == 0 {
		return
	}

	remoteIDs := make([]string, len(messageIDs))
	for i, id := range messageIDs {
		remoteIDs[i] = strconv.Itoa(id)
	}

	sink.Deleted(ctx, c.account.ID, remote, remoteIDs)
}

// unread reports Telegram's unread count for a known conversation, after
// the user read some of it here or on another device.
func (c *Connector) unread(ctx context.Context, sink connector.Sink, key string, count int) {
	if remote, ok := c.lookup(key); ok {
		sink.Unread(ctx, c.account.ID, remote, count)
	}
}

// newMessage reports a message, first reporting its conversation so a
// brand-new chat is not dropped. Our own messages from other devices are
// history: they never notify.
func (c *Connector) newMessage(ctx context.Context, sink connector.Sink, m tg.MessageClass, te tg.Entities) {
	msg, ok := m.(*tg.Message)
	if !ok {
		return
	}

	e := fromUpdate(te)
	conv, ok := peerConversation(c.account.ID, msg.PeerID, e)
	if !ok {
		remote, known := c.lookup(shortKey(msg.PeerID))
		if !known {
			return
		}
		conv.RemoteID = remote
	} else {
		c.learn(conv.RemoteID)
		sink.Conversation(ctx, conv)
	}

	if msg.Out {
		sink.History(ctx, c.account.ID, conv.RemoteID, message(msg, e))
		return
	}

	sink.Incoming(ctx, c.account.ID, conv.RemoteID, message(msg, e))
}

// readUpTo marks our messages in a conversation read, up to Telegram's
// message id maxID.
func (c *Connector) readUpTo(ctx context.Context, sink connector.Sink, key string, maxID int) {
	remote, ok := c.lookup(key)
	if !ok {
		return
	}

	c.mu.Lock()
	var read []string
	for k, localID := range c.sent {
		i := strings.LastIndex(k, "/")
		conv, id := k[:i], k[i+1:]
		n, err := strconv.Atoi(id)
		if conv == remote && err == nil && n <= maxID {
			read = append(read, localID)
			delete(c.sent, k)
		}
	}
	c.mu.Unlock()

	for _, localID := range read {
		sink.OutgoingStatus(ctx, localID, "", domain.StatusRead)
	}
}

// typing reports someone typing in a direct chat.
func (c *Connector) typing(ctx context.Context, sink connector.Sink, u *tg.UpdateUserTyping) {
	remote, ok := c.lookup("user:" + strconv.FormatInt(u.UserID, 10))
	if !ok {
		return
	}

	_, stopped := u.Action.(*tg.SendMessageCancelAction)
	sink.Typing(ctx, c.account.ID, remote, "", !stopped)
}

// fromUpdate turns an update's entities into ours.
func fromUpdate(te tg.Entities) entities {
	return entities{users: te.Users, chats: te.Chats, channels: te.Channels}
}
