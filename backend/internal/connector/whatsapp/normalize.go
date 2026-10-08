package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// selfChatTitle is the fixed title for the account's own chat with
// itself, WhatsApp's "Message yourself": it carries no useful contact
// or push name of its own (nobody has themselves saved as a contact),
// so a resolved name is never attempted for it.
const selfChatTitle = "Message yourself"

// errBadRemoteID reports a conversation or contact id this connector did
// not make: not a canonical WhatsApp JID string.
var errBadRemoteID = errors.New("whatsapp: not a canonical id")

// remoteID is the canonical form a WhatsApp JID is known by outside this
// package, for both a conversation and the person who sent a message:
// the bare "user@server" string WhatsApp itself uses, with any
// per-device suffix stripped, since a conversation or a contact is the
// same one no matter which of their devices sent a message.
//
// For a JID on the default server, the user part is the contact's phone
// number, so a remote id must never be written to a log: every place
// that reports progress logs a conversation's own store id or a count
// instead (see jidFromRemoteID for the matching read-back rule).
func remoteID(jid types.JID) string {
	return jid.ToNonAD().String()
}

// chatID is the canonical, stable remote id for the conversation a
// message, a history sync entry, a receipt, a revoke or delete, or an
// organizing change addresses. The account's own self-chat always
// collapses to one id, dev.selfChatID(), no matter whether this
// particular report named it by phone JID (PN) or by LID, so every
// linked device's messages, receipts, reactions and pin or archive
// changes land in the one "Message yourself" conversation (its phone
// JID is reliably known once paired, unlike its LID, which is why that
// one case needs no stored choice of its own).
//
// Any other chat keeps whichever remote id it was first reported
// under for the rest of its life. WhatsApp addresses the same 1:1 chat
// by PN or by LID depending on which of its own internal paths
// delivered a given report, and whatsmeow only learns the mapping
// between a contact's two forms at some point after it starts seeing
// them, not before: a chat first reported by LID, during history sync
// say, then again later by PN once the mapping arrives (or the other
// order), must resolve to the same conversation every time, never
// split into a second one under the form that happened to be known at
// the moment of a particular report. media's chat_aliases table (see
// chatalias.go) is what remembers that choice: the first time chatID
// is ever asked about this chat, under either of its two known forms,
// it records that form (or both, if the mapping is already known by
// then) as aliases of whichever one chatID returns, so every later
// call, by either form, resolves back to the exact same id, including
// after a restart (see docs/decisions.md).
//
// media being nil, such as a connector under test built without one,
// falls back to a single-call guess with no persisted choice to keep:
// the mapped phone form when whatsmeow already knows it, the bare id
// otherwise, same as before this id was made stable.
func chatID(ctx context.Context, dev device, media *mediaStore, jid types.JID) string {
	if dev.isSelfChat(ctx, jid) {
		return dev.selfChatID()
	}

	own := remoteID(jid)
	if media == nil {
		return remoteID(phoneFormOf(ctx, dev, jid))
	}

	aliases := []string{own}
	if alt := dev.altJID(ctx, jid); !alt.IsEmpty() {
		aliases = append(aliases, remoteID(alt))
	}

	canonical, err := media.resolveChatAlias(ctx, aliases)
	if err != nil || canonical == "" {
		canonical = own // first time this chat is seen at all: the form it arrived in becomes its id for good
	}

	for _, alias := range aliases {
		_ = media.putChatAlias(ctx, alias, canonical) // best effort: a save failure only means this alias is resolved fresh again next time, not that the chat splits
	}

	return canonical
}

// personID is the canonical remote id for the person who sent a
// reaction, cast a poll vote, or whose receipt is being tracked
// participant by participant in a group: jid's own mapped phone form
// when whatsmeow's LID store already knows it, jid's own remote id
// otherwise. Unlike chatID, this needs no stored choice of its own to
// stay stable: whatsmeow's LID-to-PN mapping, once learned, never
// becomes unknown again, so every call from then on agrees on the
// phone form without anything to persist. It is deliberately not used
// for message_keys.sender_id (see keys.go's senderKeyID): that value
// also has to build the exact Participant WhatsApp expects when
// reacting to, replying to or deleting someone else's message in a
// group, which whatsmeow builds from the sender address exactly as the
// message carried it, not a resolved alternative (see
// docs/decisions.md).
func personID(ctx context.Context, dev device, jid types.JID) string {
	return remoteID(phoneFormOf(ctx, dev, jid))
}

// phoneFormOf is jid's own phone-JID form when whatsmeow's LID store
// already maps it to one, or jid unchanged otherwise. personID and
// titleFallback's caller (see resolveDirectTitle) both need this exact
// check, for different reasons: one so the same person is never
// treated as two different senders, the other so a chat's fallback
// title reads like a phone number the moment that mapping becomes
// known, rather than staying a meaningless hidden id forever.
func phoneFormOf(ctx context.Context, dev device, jid types.JID) types.JID {
	if alt := dev.altJID(ctx, jid); alt.Server == types.DefaultUserServer {
		return alt
	}

	return jid
}

// jidFromRemoteID parses a stored remote id back into the JID whatsmeow
// takes, accepting only a real user on a server, in the exact canonical
// string remoteID produces. Like the Telegram connector's inputPeer,
// this guards against a non-canonical id that would parse to the right
// JID but round-trip to a different string, which would break a
// comparison done elsewhere by string equality, such as recognising a
// redelivered message.
func jidFromRemoteID(id string) (types.JID, error) {
	jid, err := types.ParseJID(id)
	if err != nil || jid.User == "" || remoteID(jid) != id {
		return types.JID{}, fmt.Errorf("%w (server %s)", errBadRemoteID, serverOf(id))
	}

	return jid, nil
}

// serverOf is the server segment of a remote id, for an error message
// that must never include the user segment: on WhatsApp's default
// server that segment is the contact's phone number.
func serverOf(id string) string {
	if i := strings.LastIndexByte(id, '@'); i != -1 {
		return id[i+1:]
	}

	return "unknown"
}

// kindFor reports whether a JID names a group or a direct chat.
func kindFor(jid types.JID) string {
	if jid.Server == types.GroupServer {
		return domain.KindGroup
	}

	return domain.KindDirect
}

// muted reports whether a mute that ends at endTime, in Unix seconds, is
// still in effect; 0 means never muted.
func muted(endTime uint64, now time.Time) bool {
	return endTime > 0 && int64(endTime) > now.Unix()
}

// conversationFromSync turns one conversation of a history sync into
// ours, with its unread count, mute and name, or reports false when it
// carries no parseable id. Pinned and archived are deliberately not
// read from here at all: this sync's own snapshot of either can lag
// behind WhatsApp's own chat settings store, which is the only place
// syncConversation reads them from (see history.go's
// reportSyncedOrganize and docs/decisions.md).
func conversationFromSync(accountID string, c *waHistorySync.Conversation, now time.Time) (domain.Conversation, bool) {
	jid, err := types.ParseJID(c.GetID())
	if err != nil || jid.IsEmpty() {
		return domain.Conversation{}, false
	}

	conv := domain.Conversation{
		AccountID: accountID,
		RemoteID:  remoteID(jid),
		Kind:      kindFor(jid),
		Title:     conversationName(c),
		Unread:    int(c.GetUnreadCount()),
		Muted:     muted(c.GetMuteEndTime(), now),
	}

	if conv.Kind == domain.KindGroup {
		conv.Members = len(c.GetParticipant())
	}

	return conv, true
}

// conversationName is a synced conversation's title: its group display
// name, its older group name field, or "" when WhatsApp reported
// neither, which a contact or push name sync fills in later.
func conversationName(c *waHistorySync.Conversation) string {
	switch {
	case c.GetDisplayName() != "":
		return c.GetDisplayName()
	case c.GetName() != "":
		return c.GetName()
	default:
		return ""
	}
}

// titleFallback is a direct chat's title when nothing else names it
// yet: WhatsApp's own fixed name for its "0" system account, the
// contact's phone number for anything else on WhatsApp's default
// server, the way WhatsApp's own clients title an unsaved contact, or,
// for a JID addressed only by its hidden id (a LID), which carries no
// phone number to show and whose digits are not one, a neutral label
// rather than a meaningless number.
func titleFallback(jid types.JID) string {
	switch {
	case jid == types.PSAJID:
		return "WhatsApp"
	case jid.Server == types.DefaultUserServer:
		return "+" + jid.User
	default:
		return "Unknown contact"
	}
}

// isSystemJID reports whether jid names something history sync or a
// live event can deliver that is not a conversation with anyone at
// all: the status broadcast, an old-style broadcast list, or a
// newsletter channel. None of these are worth showing as a chat. The
// "0" system account is not one of these: it sends real security
// notices and announcements, so it is shown as an ordinary, if
// read-only in practice, conversation titled "WhatsApp" (see
// titleFallback) rather than dropped.
func isSystemJID(jid types.JID) bool {
	switch {
	case jid == types.StatusBroadcastJID:
		return true
	case jid.IsBroadcastList():
		return true
	case jid.Server == types.NewsletterServer:
		return true
	default:
		return false
	}
}

// contactDisplayName is WhatsApp's own name for a resolved contact, in
// its own priority order: the name saved for them, their first name
// alone, their verified business name, or their self-chosen push name.
// It returns "" when info names no one, or holds none of these.
func contactDisplayName(info types.ContactInfo) string {
	switch {
	case info.FullName != "":
		return info.FullName
	case info.FirstName != "":
		return info.FirstName
	case info.BusinessName != "":
		return info.BusinessName
	default:
		return info.PushName
	}
}

// contactFromPushName turns one push name of a history sync into a
// contact, or reports false when it names no one.
func contactFromPushName(accountID string, p *waHistorySync.Pushname) (domain.Contact, bool) {
	jid, err := types.ParseJID(p.GetID())
	if err != nil || jid.IsEmpty() || p.GetPushname() == "" {
		return domain.Contact{}, false
	}

	return domain.Contact{AccountID: accountID, RemoteID: remoteID(jid), Name: p.GetPushname()}, true
}
