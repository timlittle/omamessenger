package whatsapp

// This file handles events.HistorySync, the one-off blob WhatsApp sends
// after pairing (and again, smaller, from time to time): the account's
// contacts, its conversations with their pinned, archived and muted
// state, and each conversation's recent messages. It covers every sync
// type whatsmeow reports (initial bootstrap, full, recent, push names
// and on-demand) the same way, since each carries the same shape of
// data, just a different slice of it.

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupNameTimeout bounds asking WhatsApp for a group's name, so one
// unreachable group can never hold up the rest of a sync.
const groupNameTimeout = 5 * time.Second

// handleHistorySync reports a history sync's contacts, then its
// conversations and their messages.
func (c *Connector) handleHistorySync(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.HistorySync) {
	data := e.Data

	c.syncPushnames(ctx, sink, data.GetPushnames())
	for _, sc := range data.GetConversations() {
		c.syncConversation(ctx, sink, dev, media, sc)
	}
}

// syncPushnames reports a contact for every push name a sync carries,
// and remembers each one for titling direct chats that otherwise have
// no name of their own.
func (c *Connector) syncPushnames(ctx context.Context, sink connector.Sink, names []*waHistorySync.Pushname) {
	for _, p := range names {
		contact, ok := contactFromPushName(c.account.ID, p)
		if !ok {
			continue
		}

		c.setName(contact.RemoteID, contact.Name)
		sink.Contact(ctx, contact)
	}
}

// syncConversation reports one synced conversation, its pinned and
// archived state, its recent messages and WhatsApp's own unread count
// for it, dropping it when its id does not parse.
func (c *Connector) syncConversation(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, sc *waHistorySync.Conversation) {
	jid, err := types.ParseJID(sc.GetID())
	if err != nil || jid.IsEmpty() {
		return
	}

	conv, ok := conversationFromSync(c.account.ID, sc, time.Now())
	if !ok {
		return
	}
	conv.Title = c.resolveTitle(ctx, dev, jid, conv)
	sink.Conversation(ctx, conv)

	state := c.setOrganized(conv.RemoteID, &conv.Pinned, &conv.Archived)
	sink.Organized(ctx, c.account.ID, conv.RemoteID, state.pinned, state.archived)

	for _, hm := range sc.GetMessages() {
		c.syncMessage(ctx, sink, media, conv.RemoteID, hm)
	}

	sink.Unread(ctx, c.account.ID, conv.RemoteID, conv.Unread)
}

// syncMessage reports one of a conversation's synced messages and
// remembers its media reference, if it has one, for a later download.
// convRemoteID parses back to the chat JID historyMessage needs, which
// is always possible: it came from parsing that same id in
// syncConversation.
func (c *Connector) syncMessage(ctx context.Context, sink connector.Sink, media *mediaStore, convRemoteID string, hm *waHistorySync.HistorySyncMsg) {
	chat, err := jidFromRemoteID(convRemoteID)
	if err != nil {
		return
	}

	m, ok := historyMessage(chat, hm)
	if !ok {
		return
	}

	saveMediaRef(ctx, media, convRemoteID, m.RemoteID, hm.GetMessage().GetMessage())
	sink.History(ctx, c.account.ID, convRemoteID, m)
}

// resolveTitle is a conversation's best title: the name the sync itself
// carried, a group's name fetched and cached when the sync left it
// blank, a direct chat's already-known contact or push name, or, with
// nothing else known yet, its phone number, the way WhatsApp's own
// clients title an unsaved contact. A conversation with no title at all
// would be dropped rather than shown, so this never returns "".
func (c *Connector) resolveTitle(ctx context.Context, dev device, jid types.JID, conv domain.Conversation) string {
	if conv.Title != "" {
		return conv.Title
	}

	if conv.Kind == domain.KindGroup {
		return c.resolveGroupName(ctx, dev, jid)
	}

	if name := c.nameFor(conv.RemoteID); name != "" {
		return name
	}

	return phoneTitle(jid)
}

// resolveGroupName is a group's current name, from this connector's own
// cache or, failing that, a bounded request to WhatsApp, falling back
// to a generic label rather than ever leaving a group untitled.
func (c *Connector) resolveGroupName(ctx context.Context, dev device, jid types.JID) string {
	remote := remoteID(jid)
	if name := c.nameFor(remote); name != "" {
		return name
	}

	lookupCtx, cancel := context.WithTimeout(ctx, groupNameTimeout)
	defer cancel()

	name, err := dev.groupName(lookupCtx, jid)
	if err != nil || name == "" {
		return "Group"
	}

	c.setName(remote, name)

	return name
}
