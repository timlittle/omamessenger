package whatsapp

import (
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
// ours, with its unread count, pinned and archived state, mute and
// name, or reports false when it carries no parseable id.
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
		Pinned:    c.GetPinned() > 0,
		Archived:  c.GetArchived(),
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
// yet: the contact's phone number, the way WhatsApp's own clients title
// an unsaved contact, for a JID on WhatsApp's default server. A JID
// addressed only by its hidden id (a LID) carries no phone number to
// show, and its digits are not one, so formatting it the same way would
// show a meaningless number instead of a name; it gets a neutral label
// instead.
func titleFallback(jid types.JID) string {
	if jid.Server == types.DefaultUserServer {
		return "+" + jid.User
	}

	return "Unknown contact"
}

// isSystemJID reports whether jid names something history sync or a
// live event can deliver that is not a conversation with a person or a
// group: the status broadcast, an old-style broadcast list, a
// newsletter channel, or WhatsApp's own "0" system account. None of
// these are worth showing as a chat, and "0" in particular would
// otherwise title as the meaningless "+0".
func isSystemJID(jid types.JID) bool {
	switch {
	case jid == types.StatusBroadcastJID, jid == types.PSAJID:
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
