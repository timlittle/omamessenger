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

// syncSource bundles what every step of a history sync needs besides
// the sink and the data being synced: the device to resolve names and
// fetch group info from, and the media store to remember attachment
// references in. Bundling the two keeps syncConversation and
// syncMessage within this codebase's argument-count limit.
type syncSource struct {
	dev   device
	media *mediaStore
}

// handleHistorySync reports a history sync's contacts, then its
// conversations and their messages.
func (c *Connector) handleHistorySync(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.HistorySync) {
	data := e.Data
	src := syncSource{dev: dev, media: media}

	c.syncPushnames(ctx, sink, data.GetPushnames())
	for _, sc := range data.GetConversations() {
		c.syncConversation(ctx, sink, src, sc)
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

		c.rememberName(contact.RemoteID, contact.Name, nameRankPushName)
		sink.Contact(ctx, contact)
	}
}

// syncConversation reports one synced conversation, its pinned and
// archived state, its recent messages and WhatsApp's own unread count
// for it, dropping it when its id does not parse or it names something
// that is not a real conversation (see isSystemJID).
func (c *Connector) syncConversation(ctx context.Context, sink connector.Sink, src syncSource, sc *waHistorySync.Conversation) {
	jid, err := types.ParseJID(sc.GetID())
	if err != nil || jid.IsEmpty() || isSystemJID(jid) {
		return
	}

	conv, ok := conversationFromSync(c.account.ID, sc, time.Now())
	if !ok {
		return
	}
	conv.Title, conv.Members = c.resolveConversation(ctx, src.dev, jid, conv)
	c.reportConversation(ctx, sink, conv)

	state := c.setOrganized(conv.RemoteID, &conv.Pinned, &conv.Archived)
	sink.Organized(ctx, c.account.ID, conv.RemoteID, state.pinned, state.archived)

	var incoming []domain.Message
	for _, hm := range sc.GetMessages() {
		if m, ok := c.syncMessage(ctx, sink, src, conv.RemoteID, hm); ok && !m.Outgoing {
			incoming = append(incoming, m)
		}
	}
	c.noteUnreadTail(conv.RemoteID, incoming, conv.Unread)

	sink.Unread(ctx, c.account.ID, conv.RemoteID, conv.Unread)
}

// syncMessage reports one of a conversation's synced messages and
// remembers its media reference, if it has one, for a later download,
// returning the message so syncConversation can work out which of them
// are still unread, or false for a message with no content of its own
// to show (see isContentless). convRemoteID parses back to the chat JID
// historyMessage needs, which is always possible: it came from parsing
// that same id in syncConversation.
func (c *Connector) syncMessage(ctx context.Context, sink connector.Sink, src syncSource, convRemoteID string, hm *waHistorySync.HistorySyncMsg) (domain.Message, bool) {
	chat, err := jidFromRemoteID(convRemoteID)
	if err != nil {
		return domain.Message{}, false
	}

	m, ok := historyMessage(ctx, src.dev, chat, hm)
	if !ok {
		return domain.Message{}, false
	}

	saveMediaRef(ctx, src.media, convRemoteID, m.RemoteID, hm.GetMessage().GetMessage())
	info := historyMessageInfo(chat, hm.GetMessage())
	saveMessageKey(ctx, src.media, convRemoteID, m.RemoteID, messageKey{senderID: senderKeyID(info), fromMe: m.Outgoing})
	sink.History(ctx, c.account.ID, convRemoteID, m)

	return m, true
}

// noteUnreadTail marks the newest unread incoming messages of a synced
// conversation as pending read, so a MarkRead for it, once the user
// opens it, tells WhatsApp about messages that arrived before this
// process ever connected live and so were never noted any other way.
// History sync carries each conversation's unread count but not which
// of its messages are unread, so the newest unread incoming messages,
// in the order the sync listed them, are taken as the best information
// available.
func (c *Connector) noteUnreadTail(convRemoteID string, incoming []domain.Message, unread int) {
	if unread > len(incoming) {
		unread = len(incoming)
	}

	for _, m := range incoming[len(incoming)-unread:] {
		c.notePendingRead(convRemoteID, m.SenderID, m.RemoteID)
	}
}

// resolveConversation is conv's best title and, for a group, its member
// count: the account's own self-chat label, the name and participants
// the sync itself carried, a group's name and member count fetched and
// cached when the sync left both blank, a direct chat's already-known
// contact or push name, or, with nothing else known yet, a fallback
// title. A conversation with no title at all would be dropped rather
// than shown, so this never returns "".
func (c *Connector) resolveConversation(ctx context.Context, dev device, jid types.JID, conv domain.Conversation) (title string, members int) {
	if dev.isSelfChat(jid) {
		return selfChatTitle, 0
	}

	if conv.Kind == domain.KindGroup {
		return c.resolveGroupName(ctx, dev, jid, conv.Title, conv.Members)
	}

	if conv.Title != "" {
		return c.rememberName(conv.RemoteID, conv.Title, nameRankContact), 0
	}

	return c.resolveDirectTitle(ctx, dev, jid, ""), 0
}

// resolveDirectTitle is a direct chat's best title: its contact's
// resolved name (mapping a LID to its phone JID first, see
// device.contactName), the best name already cached for it, the push
// name this report itself carries, or, with nothing else known, a
// fallback title. It never formats a LID as if it were a phone number.
func (c *Connector) resolveDirectTitle(ctx context.Context, dev device, jid types.JID, pushName string) string {
	if name := dev.contactName(ctx, jid); name != "" {
		return c.rememberName(remoteID(jid), name, nameRankContact)
	}

	if name := c.nameFor(remoteID(jid)); name != "" {
		return name
	}

	if pushName != "" {
		return c.rememberName(remoteID(jid), pushName, nameRankPushName)
	}

	return titleFallback(jid)
}

// resolveGroupName is a group's current name and member count: whichever
// the sync itself carried or this connector's own cache already holds,
// or, failing both, a single bounded request to WhatsApp that resolves
// them together, cached so a later sync or message for the same group
// never asks again. It falls back to a generic label rather than ever
// leaving a group untitled.
func (c *Connector) resolveGroupName(ctx context.Context, dev device, jid types.JID, syncedTitle string, syncedMembers int) (string, int) {
	remote := remoteID(jid)

	members := syncedMembers
	switch {
	case members > 0:
		c.setGroupMembers(remote, members)
	default:
		if cached, ok := c.groupMembersFor(remote); ok {
			members = cached
		}
	}

	name := syncedTitle
	if name == "" {
		name = c.nameFor(remote)
	}
	if name != "" {
		return c.rememberName(remote, name, nameRankContact), members
	}

	lookupCtx, cancel := context.WithTimeout(ctx, groupNameTimeout)
	defer cancel()

	fetchedName, fetchedMembers, err := dev.groupInfo(lookupCtx, jid)
	if err != nil || fetchedName == "" {
		return "Group", members
	}

	if syncedMembers == 0 {
		if _, cached := c.groupMembersFor(remote); !cached {
			members = fetchedMembers
			c.setGroupMembers(remote, members)
		}
	}

	return c.rememberName(remote, fetchedName, nameRankContact), members
}
