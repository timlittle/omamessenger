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
// references in. Bundling the two keeps syncConversation within this
// codebase's argument-count limit.
type syncSource struct {
	dev   device
	media *mediaStore
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
// that is not a real conversation (see isSystemJID). A conversation
// this connector has never reported before, in a sync batch with
// nothing but contentless messages (or none at all), is dropped too:
// WhatsApp's history sync carries entries for chats with no content
// worth showing (a group's invite link that was never opened, a LID
// shadow of a chat already known by its phone JID), which otherwise
// littered the list with an unresolved name and no real preview. Once
// this connector has reported a conversation for real, a later sync
// that only updates its pinned, archived or unread state still reaches
// it, even with no new messages of its own.
func (c *Connector) syncConversation(ctx context.Context, sink connector.Sink, src syncSource, sc *waHistorySync.Conversation) {
	jid, err := types.ParseJID(sc.GetID())
	if err != nil || jid.IsEmpty() || isSystemJID(jid) {
		return
	}

	conv, ok := conversationFromSync(c.account.ID, sc, time.Now())
	if !ok {
		return
	}
	conv.RemoteID = chatID(ctx, src.dev, jid)

	if !c.knownChat(conv.RemoteID) && !hasRealContent(sc.GetMessages()) {
		logSyncedConversationDropped(jid, sc)
		return
	}

	conv.Title, conv.Members = c.resolveConversation(ctx, src.dev, jid, conv, sc)
	c.reportConversation(ctx, sink, conv)
	c.reportSyncedOrganize(ctx, sink, conv)

	target := syncTarget{syncSource: src, chat: jid, convRemoteID: conv.RemoteID}
	for _, hm := range sc.GetMessages() {
		c.syncMessage(ctx, sink, target, hm)
	}

	sink.Unread(ctx, c.account.ID, conv.RemoteID, conv.Unread)
}

// reportSyncedOrganize reports conv's pinned and archived state from
// this sync, unless this process has pinned or archived it more
// recently than any live echo has confirmed (see isLocalOrganize): a
// sync's own snapshot can lag a patch this process just sent (see
// organize.go's SetPinned and SetArchived), and reporting it anyway
// would revert a local change WhatsApp has not caught up with yet.
//
// A field a live Pin or Archive event has already confirmed from app
// state (see setOrganizedFromAppState) is left out of the merge
// entirely, field by field, rather than only skipped while a local
// change is pending: pinned and archived live mostly in app state, not
// in this sync's own Conversation fields (a chat pinned purely through
// app state carries no pin timestamp here), so once a live echo has
// confirmed one, a sync's own snapshot of it must never be trusted
// again for the rest of this run. This still reports the conversation's
// up to date, merged state either way, so an app-state pin or archive
// that arrived before this conversation ever existed, and so could not
// be applied at the time (see connector.go's setOrganized), is applied
// now that it does.
func (c *Connector) reportSyncedOrganize(ctx context.Context, sink connector.Sink, conv domain.Conversation) {
	if c.isLocalOrganize(conv.RemoteID) {
		return
	}

	pinnedKnown, archivedKnown := c.organizeAppStateKnown(conv.RemoteID)
	pinned, archived := &conv.Pinned, &conv.Archived
	if pinnedKnown {
		pinned = nil
	}
	if archivedKnown {
		archived = nil
	}

	state := c.setOrganized(conv.RemoteID, pinned, archived)
	sink.Organized(ctx, c.account.ID, conv.RemoteID, state.pinned, state.archived)
}

// hasRealContent reports whether at least one of a conversation's
// synced messages carries something a person actually sent, as
// opposed to every one of them being one of WhatsApp's own protocol
// notices, a reaction, an edit or a revoke (see isContentless and
// isReaction/isRevoke/isEdit in normalize_events.go), none of which
// ever become a stored message of their own.
func hasRealContent(hms []*waHistorySync.HistorySyncMsg) bool {
	for _, hm := range hms {
		content := hm.GetMessage().GetMessage()
		if content != nil && !isReaction(content) && !isRevoke(content) && !isEdit(content) && !isContentless(content) {
			return true
		}
	}

	return false
}

// logSyncedConversationDropped reports a conversation history sync
// never reported before, dropped because none of its synced messages
// carried anything a person actually sent (see hasRealContent): as
// logSystemChatDropped, with the first contentless message's own
// field names, when jid is WhatsApp's own "0" system account, so a
// report that its announcements and security notices never appear can
// be checked against this specific reason, or as an ordinary
// logDropped otherwise.
func logSyncedConversationDropped(jid types.JID, sc *waHistorySync.Conversation) {
	if jid != types.PSAJID {
		logDropped(reasonNoRealContent)
		return
	}

	logSystemChatDropped(reasonNoRealContent, syncedContentFields(sc))
}

// syncedContentFields is the field-name paths (see fieldPaths) of the
// first of sc's synced messages that carries any content at all, or ""
// when sc has no messages, for logSyncedConversationDropped.
func syncedContentFields(sc *waHistorySync.Conversation) string {
	for _, hm := range sc.GetMessages() {
		if content := hm.GetMessage().GetMessage(); content != nil {
			return fieldPaths(content)
		}
	}

	return ""
}

// syncMessage reports one of a conversation's synced messages and
// remembers its media reference, if it has one, for a later download,
// doing nothing for a message with no content of its own to show (see
// isContentless), which it logs the same way a live message's drop
// is (see logContentlessDrop); a reaction, edit or revoke is dropped
// here too, but silently, since those are not really missing: the
// live events that cover them already reported them when this
// account was connected to receive them, and a bulk sync never
// replays them again. Its message_keys row (see keys.go), saved the
// same way a live message's is, is what later lets MarkRead pick this
// message out of the conversation's newest ones, even after a restart
// (see markread.go): history sync carries each conversation's unread
// count but not which of its messages are unread, so MarkRead chooses
// for itself once it knows that count.
func (c *Connector) syncMessage(ctx context.Context, sink connector.Sink, target syncTarget, hm *waHistorySync.HistorySyncMsg) {
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

		return
	}
	m = c.improvedSenderName(m)

	saveMediaRef(ctx, target.media, target.convRemoteID, m.RemoteID, content)
	info := historyMessageInfo(target.chat, hm.GetMessage())
	saveMessageKey(ctx, target.media, target.convRemoteID, m.RemoteID, messageKey{senderID: senderKeyID(info), fromMe: m.Outgoing, timestamp: m.Created})
	savePoll(ctx, target.media, target.convRemoteID, m.RemoteID, content)
	sink.History(ctx, c.account.ID, target.convRemoteID, m)
}

// resolveConversation is conv's best title and, for a group, its member
// count: the account's own self-chat label, the name and participants
// the sync itself carried, a group's name and member count fetched and
// cached when the sync left both blank, a direct chat's already-known
// contact or push name, a business's own verified name carried by one
// of sc's own synced messages, or, with nothing else known yet, a
// fallback title. A conversation with no title at all would be
// dropped rather than shown, so this never returns "".
func (c *Connector) resolveConversation(ctx context.Context, dev device, jid types.JID, conv domain.Conversation, sc *waHistorySync.Conversation) (title string, members int) {
	if dev.isSelfChat(ctx, jid) {
		return selfChatTitle, 0
	}

	if conv.Kind == domain.KindGroup {
		return c.resolveGroupName(ctx, dev, jid, conv.Title, conv.Members)
	}

	if conv.Title != "" {
		return c.rememberName(conv.RemoteID, conv.Title, nameRankContact), 0
	}

	return c.resolveDirectTitle(ctx, dev, jid, "", syncedBusinessName(sc)), 0
}

// resolveDirectTitle is a direct chat's best title: its contact's
// resolved name (mapping a LID to its phone JID first, see
// device.contactName; this already covers a business whose verified
// name is saved in the contact store, see contactDisplayName), the
// best name already cached for it, the business's own verified name
// this particular report carries, the push name it carries, or, with
// nothing else known, a fallback title. It never formats a LID as if
// it were a phone number.
func (c *Connector) resolveDirectTitle(ctx context.Context, dev device, jid types.JID, pushName, businessName string) string {
	if name := dev.contactName(ctx, jid); name != "" {
		return c.rememberName(remoteID(jid), name, nameRankContact)
	}

	if name := c.nameFor(remoteID(jid)); name != "" {
		return name
	}

	if businessName != "" {
		return c.rememberName(remoteID(jid), businessName, nameRankBusiness)
	}

	if pushName != "" {
		return c.rememberName(remoteID(jid), pushName, nameRankPushName)
	}

	return titleFallback(jid)
}

// resolveGroupName is a group's current name and member count: whichever
// the sync itself carried or this connector's own cache already holds,
// or, failing either one, a single bounded request to WhatsApp that
// resolves both together, cached so a later sync or message for the
// same group never asks again. A name already known never needs that
// request on its own, but an unknown member count still does, even
// when the name came from the sync: a synced conversation often
// carries a group's display name with no participant list at all, and
// without this a group titled straight from history sync would be
// stuck showing no members until something else happened to ask.
// Resolving falls back to a generic label rather than ever leaving a
// group untitled.
func (c *Connector) resolveGroupName(ctx context.Context, dev device, jid types.JID, syncedTitle string, syncedMembers int) (string, int) {
	remote := remoteID(jid)
	members, membersKnown := c.cachedGroupMembers(remote, syncedMembers)

	name := syncedTitle
	if name == "" {
		name = c.nameFor(remote)
	}
	if name != "" && membersKnown {
		return c.rememberName(remote, name, nameRankContact), members
	}

	lookupCtx, cancel := context.WithTimeout(ctx, groupNameTimeout)
	defer cancel()

	fetchedName, fetchedMembers, err := dev.groupInfo(lookupCtx, jid)
	if err == nil && !membersKnown {
		members = fetchedMembers
		c.setGroupMembers(remote, members)
	}
	if name == "" && err == nil {
		name = fetchedName
	}
	if name == "" {
		return "Group", members
	}

	return c.rememberName(remote, name, nameRankContact), members
}

// cachedGroupMembers is a group's member count from the sync itself, or
// this connector's own cache, and whether either source actually knows
// it yet: a group always has at least one other member, so a cached 0
// only ever means "WhatsApp was asked and said so", never "never
// asked".
func (c *Connector) cachedGroupMembers(remote string, syncedMembers int) (members int, known bool) {
	if syncedMembers > 0 {
		c.setGroupMembers(remote, syncedMembers)
		return syncedMembers, true
	}

	cached, ok := c.groupMembersFor(remote)

	return cached, ok
}
