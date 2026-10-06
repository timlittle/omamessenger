package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Sync limits: the dialogs fetched at start, and the recent messages of
// each. Older messages load when the user scrolls back.
const (
	dialogLimit  = 100
	historyLimit = 30
)

// sync reports the account's contacts and dialogs, then each dialog's
// recent messages. Every dialog is listed first, with the last message
// Telegram sends alongside it, so a dialog whose history fails to load,
// such as a channel the user has left, still shows and holds up no other.
func (c *Connector) sync(ctx context.Context, api *tg.Client, sink connector.Sink) error {
	if err := c.syncContacts(ctx, api, sink); err != nil {
		return err
	}

	result, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{OffsetPeer: &tg.InputPeerEmpty{}, Limit: dialogLimit})
	if err != nil {
		return fmt.Errorf("telegram: dialogs: %w", err)
	}

	dialogs, ok := result.AsModified()
	if !ok {
		return nil
	}

	listed := c.listDialogs(ctx, sink, dialogs)
	for _, l := range listed {
		if err := c.syncHistory(ctx, api, sink, l.conv, l.dialog); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return nil
}

// listedDialog is a dialog already reported as a conversation.
type listedDialog struct {
	conv   domain.Conversation
	dialog *tg.Dialog
}

// listDialogs reports each dialog as a conversation with its last message
// and Telegram's unread count, returning the ones it reported.
func (c *Connector) listDialogs(ctx context.Context, sink connector.Sink, dialogs tg.ModifiedMessagesDialogs) []listedDialog {
	e := newEntities(dialogs.GetUsers(), dialogs.GetChats())
	top := map[string]*tg.Message{}
	for _, m := range dialogs.GetMessages() {
		if msg, ok := m.(*tg.Message); ok {
			top[shortKey(msg.PeerID)+"/"+strconv.Itoa(msg.ID)] = msg
		}
	}

	var listed []listedDialog
	for _, d := range dialogs.GetDialogs() {
		dialog, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}

		conv, ok := conversation(c.account.ID, dialog, e, time.Now())
		if !ok {
			continue
		}

		c.learn(conv.RemoteID)
		sink.Conversation(ctx, conv)
		if msg, ok := top[shortKey(dialog.Peer)+"/"+strconv.Itoa(dialog.TopMessage)]; ok {
			sink.History(ctx, c.account.ID, conv.RemoteID, message(msg, e))
		}
		sink.Unread(ctx, c.account.ID, conv.RemoteID, dialog.UnreadCount)
		listed = append(listed, listedDialog{conv: conv, dialog: dialog})
	}

	return listed
}

// syncHistory reports a dialog's recent messages, waiting out Telegram's
// rate limit if it asks.
func (c *Connector) syncHistory(ctx context.Context, api *tg.Client, sink connector.Sink, conv domain.Conversation, d *tg.Dialog) error {
	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	request := &tg.MessagesGetHistoryRequest{Peer: peer, Limit: historyLimit}
	result, err := api.MessagesGetHistory(ctx, request)
	if waited, _ := tgerr.FloodWait(ctx, err); waited {
		result, err = api.MessagesGetHistory(ctx, request)
	}
	if err != nil {
		return fmt.Errorf("telegram: history: %w", err)
	}

	history, ok := result.AsModified()
	if !ok {
		return nil
	}

	he := newEntities(history.GetUsers(), history.GetChats())
	for _, m := range history.GetMessages() {
		if msg, ok := m.(*tg.Message); ok {
			sink.History(ctx, c.account.ID, conv.RemoteID, message(msg, he))
		}
	}

	// Telegram knows which of these were read; its count replaces the
	// history's, which would otherwise count every old message as new.
	sink.Unread(ctx, c.account.ID, conv.RemoteID, d.UnreadCount)

	return nil
}

// syncContacts reports the account's contacts.
func (c *Connector) syncContacts(ctx context.Context, api *tg.Client, sink connector.Sink) error {
	result, err := api.ContactsGetContacts(ctx, 0)
	if err != nil {
		return fmt.Errorf("telegram: contacts: %w", err)
	}

	contacts, ok := result.(*tg.ContactsContacts)
	if !ok {
		return nil
	}

	for _, u := range contacts.Users {
		if user, ok := u.(*tg.User); ok {
			sink.Contact(ctx, contact(c.account.ID, user))
		}
	}

	return nil
}

// learn records a conversation's full remote id under its short peer key,
// for updates that name a peer without its access hash.
func (c *Connector) learn(remote string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.remotes[peerKey(remote)] = remote
}

// lookup returns the full remote id for a short peer key, if known.
func (c *Connector) lookup(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	remote, ok := c.remotes[key]

	return remote, ok
}

// peerKey drops the access hash from a remote id: "user:42:99" gives
// "user:42".
func peerKey(remote string) string {
	parts := strings.SplitN(remote, ":", 3)
	if len(parts) < 2 {
		return remote
	}

	return parts[0] + ":" + parts[1]
}

// shortKey is the peer key of a Telegram peer.
func shortKey(p tg.PeerClass) string {
	switch p := p.(type) {
	case *tg.PeerUser:
		return "user:" + strconv.FormatInt(p.UserID, 10)
	case *tg.PeerChat:
		return "chat:" + strconv.FormatInt(p.ChatID, 10)
	case *tg.PeerChannel:
		return "channel:" + strconv.FormatInt(p.ChannelID, 10)
	default:
		return ""
	}
}
