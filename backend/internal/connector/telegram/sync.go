package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// Sync limits: the dialogs fetched at start, and the recent messages of
// each. Older messages load when the user scrolls back.
const (
	dialogLimit  = 100
	historyLimit = 30
)

// sync reports the account's contacts, dialogs and their recent messages.
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

	e := newEntities(dialogs.GetUsers(), dialogs.GetChats())
	for _, d := range dialogs.GetDialogs() {
		dialog, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}

		if err := c.syncDialog(ctx, api, sink, dialog, e); err != nil {
			return err
		}
	}

	return nil
}

// syncDialog reports one dialog and its recent messages.
func (c *Connector) syncDialog(ctx context.Context, api *tg.Client, sink connector.Sink, d *tg.Dialog, e entities) error {
	conv, ok := conversation(c.account.ID, d, e, time.Now())
	if !ok {
		return nil
	}

	c.learn(conv.RemoteID)
	sink.Conversation(ctx, conv)

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: historyLimit})
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
