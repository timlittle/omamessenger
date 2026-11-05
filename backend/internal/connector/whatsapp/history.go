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
)

// syncSource bundles what every step of a history sync needs besides
// the data being synced: the device to resolve names and fetch group
// info from, the media store to remember attachment references in,
// and the sink to report what a fetch resolves, such as a group's
// current disappearing timer (see resolveGroupName). Bundling these
// keeps syncConversation and resolveGroupName within this codebase's
// argument-count limit.
type syncSource struct {
	dev   device
	media *mediaStore
	sink  connector.Sink
}

// syncTarget bundles syncSource with the one conversation syncMessage
// is currently reporting messages for: the JID the sync actually
// addressed it by, which historyMessage needs to work out a direct
// message's sender, and that conversation's canonical remote id (see
// chatID), used for everything this connector stores or reports, which
// collapses to one id for the self-chat no matter which JID form chat
// is. Bundling these keeps syncMessage within this codebase's
// argument-count limit.
type syncTarget struct {
	syncSource
	chat         types.JID
	convRemoteID string
}

// handleHistorySync reports a history sync's contacts, then its
// conversations and their messages. A sync of type ON_DEMAND is WhatsApp's
// primary phone answering a LoadOlder request (see history_ondemand.go):
// once this sync has reported that chat's messages the normal way below,
// the answer also wakes whichever LoadOlder call is waiting for it, with
// how many of them were actually reported.
func (c *Connector) handleHistorySync(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.HistorySync) {
	data := e.Data
	src := syncSource{dev: dev, media: media, sink: sink}
	onDemand := data.GetSyncType() == waHistorySync.HistorySync_ON_DEMAND

	c.syncPushnames(ctx, sink, dev, data.GetPushnames())
	for _, sc := range data.GetConversations() {
		chatRemote, reported := c.syncConversation(ctx, sink, src, sc)
		if onDemand && chatRemote != "" {
			c.deliverOnDemandHistory(chatRemote, reported)
		}
	}
}

// syncPushnames reports a contact for every push name a sync carries,
// remembering each one for titling direct chats with no name of
// their own, at the rank contactFromPushName resolves for it.
func (c *Connector) syncPushnames(ctx context.Context, sink connector.Sink, dev device, names []*waHistorySync.Pushname) {
	for _, p := range names {
		contact, rank, ok := contactFromPushName(ctx, dev, c.account.ID, p)
		if !ok {
			continue
		}

		c.rememberName(contact.RemoteID, contact.Name, rank)
		sink.Contact(ctx, contact)
	}
}

// syncConversation reports one synced conversation, its pinned and
// archived state, its recent messages and WhatsApp's own unread count
// for it, dropping it only when its id does not parse or it names
// something that is not a real conversation (see isSystemJID). Every
// other chat the phone lists is shown, even when none of its synced
// messages has anything to display: it is still a chat on the phone,
// and a later message needs it to exist.
//
// It returns the conversation's canonical remote id, or "" when the
// entry was dropped before anything could be resolved, and how many of
// its synced messages were actually reported through the Sink (fewer
// than len(sc.GetMessages()) whenever some were a reaction, an edit, a
// revoke or other housekeeping content, see syncMessage). A bulk sync's
// own caller has no use for either value; handleHistorySync reads them
// to resolve a LoadOlder call waiting on this exact chat (see
// history_ondemand.go).
func (c *Connector) syncConversation(ctx context.Context, sink connector.Sink, src syncSource, sc *waHistorySync.Conversation) (string, int) {
	jid, err := types.ParseJID(sc.GetID())
	if err != nil || jid.IsEmpty() {
		logDropped("unparseable chat id")
		return "", 0
	}
	if isSystemJID(jid) {
		logDropped("system jid")
		return "", 0
	}

	conv, ok := conversationFromSync(c.account.ID, sc, time.Now())
	if !ok {
		logDropped("unparseable chat id")
		return "", 0
	}
	conv.RemoteID = chatID(ctx, src.dev, src.media, jid)

	conv.Title, conv.Members = c.resolveConversation(ctx, src, jid, conv, sc)
	c.reportConversation(ctx, sink, conv)
	c.reportSyncedOrganize(ctx, sink, src.dev, jid, conv.RemoteID)
	// A full history sync is authoritative for a chat's current
	// disappearing timer, unlike an ordinary message report, so this
	// always reports whatever the sync carries, including 0 for none.
	c.reportDisappearingTimer(ctx, sink, conv.RemoteID, int64(sc.GetEphemeralExpiration()))

	target := syncTarget{syncSource: src, chat: jid, convRemoteID: conv.RemoteID}
	reported := 0
	for _, hm := range sc.GetMessages() {
		if c.syncMessage(ctx, sink, target, hm) {
			reported++
		}
	}

	sink.Unread(ctx, c.account.ID, conv.RemoteID, conv.Unread)

	return conv.RemoteID, reported
}

// reportSyncedOrganize reports the conversation jid names pinned and
// archived exactly as WhatsApp's own chat settings store currently has
// it (see device.chatSettings), rather than anything this sync's own
// Conversation entry carries: that snapshot can lag behind a patch
// this process or the phone already made (a resync is eventually
// consistent, and a chat pinned purely through app state carries no
// pin timestamp here at all), which is what used to let a resync
// revert a pin moments after it was set. Reading the settings fresh
// here, every time, also means a pin or archive that arrived live
// before this conversation ever existed to report it to (see
// organize.go's handlePin and handleArchive) applies itself correctly
// the moment this sync finally creates it, with nothing for this
// connector to have remembered in the meantime. A read failure is
// logged and skipped, leaving organizing for this chat to the next
// sync or live echo, rather than reporting a guess.
func (c *Connector) reportSyncedOrganize(ctx context.Context, sink connector.Sink, dev device, jid types.JID, remoteID string) {
	pinned, archived, err := dev.chatSettings(ctx, jid)
	if err != nil {
		logOrganizeReadFailed(err)
		return
	}

	sink.Organized(ctx, c.account.ID, remoteID, pinned, archived)
}

// syncMessage reports one of a conversation's synced messages and
// remembers its media reference for a later download. It drops a
// message with no content to show (see isContentless), a reaction,
// edit or revoke (already covered by live events), or one whose id
// is reused by another sender (see saveIncomingKeyedData). It returns
// whether hm was actually reported, so syncConversation can count how
// many of a synced chat's messages were (see history_ondemand.go).
func (c *Connector) syncMessage(ctx context.Context, sink connector.Sink, target syncTarget, hm *waHistorySync.HistorySyncMsg) bool {
	content := hm.GetMessage().GetMessage()
	if content != nil {
		if field, ok := unknownContentKind(content); ok {
			logUnknownKind(field)
		}
	}

	m, ok := historyMessage(ctx, target.dev, target.chat, hm)
	if !ok {
		if content != nil && isContentless(content) {
			logContentlessDrop(target.chat, content)
		}

		return false
	}
	m = c.improvedSenderName(m)

	// content is still whichever ephemeral, view-once or other wrapper
	// WhatsApp stored it in (see unwrap's own doc comment: a live
	// message is already unwrapped by whatsmeow's own event dispatch
	// before this file ever sees it, but a history sync's
	// WebMessageInfo never goes through that). historyMessage already
	// unwraps its own copy to read m's text and media, so the reference
	// and poll this saves must be read from the same unwrapped content,
	// or a wrapped photo or video's download reference is silently
	// never saved at all.
	unwrapped := unwrap(content)
	info := historyMessageInfo(target.chat, hm.GetMessage())
	key := messageKey{senderID: senderKeyID(info), fromMe: m.Outgoing, timestamp: m.Created}
	// A bulk sync has no placeholder to replace, so a reused id here
	// always means drop, never report.
	keyed := incomingKeyedMessage{conversationRemoteID: target.convRemoteID, messageRemoteID: m.RemoteID, key: key, msg: unwrapped}
	if !saveIncomingKeyedData(ctx, target.media, keyed) {
		return false
	}

	sink.History(ctx, c.account.ID, target.convRemoteID, m)

	return true
}
