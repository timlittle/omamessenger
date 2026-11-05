package whatsapp

// This file isolates resolving a synced conversation's title and, for a
// group, its member count, split out of history.go.

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupNameTimeout bounds asking WhatsApp for a group's name, so one
// unreachable group can never hold up the rest of a sync.
const groupNameTimeout = 5 * time.Second

// resolveConversation is conv's best title and, for a group, its
// member count: the self-chat label, the sync's own name and
// participants, a fetched group name, a direct chat's known contact
// or push name, or a fallback title. Never returns "", since an
// untitled conversation would be dropped rather than shown.
func (c *Connector) resolveConversation(ctx context.Context, src syncSource, jid types.JID, conv domain.Conversation, sc *waHistorySync.Conversation) (title string, members int) {
	if src.dev.isSelfChat(ctx, jid) {
		return selfChatTitle, 0
	}

	if conv.Kind == domain.KindGroup {
		return c.resolveGroupName(ctx, src, jid, conv.Title, conv.Members)
	}

	if conv.Title != "" {
		name, _ := c.rememberName(conv.RemoteID, conv.Title, nameRankContact)
		return name, 0
	}

	return c.resolveDirectTitle(ctx, src, jid, syncedPushName(sc), syncedBusinessName(sc)), 0
}

// syncedPushName is the push name carried by whichever of sc's own
// synced messages first has one from someone other than this account,
// or "" when none does. An outgoing message's own PushName field
// describes this account, never the chat partner (see ensureChat's
// own rule in live.go), so one is skipped here too.
func syncedPushName(sc *waHistorySync.Conversation) string {
	for _, hm := range sc.GetMessages() {
		raw := hm.GetMessage()
		if raw.GetKey().GetFromMe() {
			continue
		}

		if name := raw.GetPushName(); name != "" {
			return name
		}
	}

	return ""
}

// resolveDirectTitle is a direct chat's best title: its contact's
// resolved name (see device.contactName), the name already cached for
// its canonical chat id, the business or push name this report
// carries, or a fallback built from its phone JID. Resolving by the
// chat's canonical id, rather than whichever address form this report
// uses, keeps a name learned under one address valid under another,
// and stops a weaker fallback from overwriting a better cached name
// (see rememberName).
func (c *Connector) resolveDirectTitle(ctx context.Context, src syncSource, jid types.JID, pushName, businessName string) string {
	remote := chatID(ctx, src.dev, src.media, jid)

	if name, fromContact := src.dev.contactName(ctx, jid); name != "" {
		resolved, _ := c.rememberName(remote, name, rankFor(fromContact))
		return resolved
	}

	if name, _ := c.nameFor(remote); name != "" {
		return name
	}

	if businessName != "" {
		resolved, _ := c.rememberName(remote, businessName, nameRankBusiness)
		return resolved
	}

	if pushName != "" {
		resolved, _ := c.rememberName(remote, pushName, nameRankPushName)
		return resolved
	}

	return titleFallback(phoneFormOf(ctx, src.dev, jid))
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
// group untitled. Whenever the request actually runs, its group's
// current disappearing timer is reported through src's sink too: this
// is the only WhatsApp API call this connector makes for a group's
// info, so a group whose name and member count are already cached
// never asks, and its timer still reaches this connector live instead
// (see live.go's handleEphemeralSetting).
func (c *Connector) resolveGroupName(ctx context.Context, src syncSource, jid types.JID, syncedTitle string, syncedMembers int) (string, int) {
	remote := remoteID(jid)
	members, membersKnown := c.cachedGroupMembers(remote, syncedMembers)

	name := syncedTitle
	if name == "" {
		name, _ = c.nameFor(remote)
	}
	if name != "" && membersKnown {
		resolved, _ := c.rememberName(remote, name, nameRankContact)
		return resolved, members
	}

	lookupCtx, cancel := context.WithTimeout(ctx, groupNameTimeout)
	defer cancel()

	fetchedName, fetchedMembers, timer, err := src.dev.groupInfo(lookupCtx, jid)
	if err == nil {
		c.reportDisappearingTimer(ctx, src.sink, remote, timer)
	}
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

	resolved, _ := c.rememberName(remote, name, nameRankContact)
	return resolved, members
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
