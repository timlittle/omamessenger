package telegram

import (
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// entities are the users, groups and channels a Telegram response
// mentions, by id, so peers in it can be named and given access hashes.
type entities struct {
	users    map[int64]*tg.User
	chats    map[int64]*tg.Chat
	channels map[int64]*tg.Channel
}

// newEntities indexes the users and chats of a response.
func newEntities(users []tg.UserClass, chats []tg.ChatClass) entities {
	e := entities{users: map[int64]*tg.User{}, chats: map[int64]*tg.Chat{}, channels: map[int64]*tg.Channel{}}
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			e.users[user.ID] = user
		}
	}

	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Chat:
			e.chats[chat.ID] = chat
		case *tg.Channel:
			e.channels[chat.ID] = chat
		}
	}

	return e
}

// peer resolves a peer to the input peer Telegram takes, using the access
// hashes the entities carry.
func (e entities) peer(p tg.PeerClass) (tg.InputPeerClass, bool) {
	switch p := p.(type) {
	case *tg.PeerUser:
		u, ok := e.users[p.UserID]
		if !ok {
			return nil, false
		}
		return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}, true
	case *tg.PeerChat:
		_, ok := e.chats[p.ChatID]
		return &tg.InputPeerChat{ChatID: p.ChatID}, ok
	case *tg.PeerChannel:
		c, ok := e.channels[p.ChannelID]
		if !ok {
			return nil, false
		}
		return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}, true
	default:
		return nil, false
	}
}

// archiveFolderID is the folder Telegram's clients file an archived chat
// under; folder 0 is the default, unarchived list.
const archiveFolderID = 1

// conversation turns a dialog into a conversation, or reports false when
// the response did not include the dialog's peer. Pinned and archived are
// not part of this: they reach the store only through Sink.Organized, so
// a later bare report of the same conversation can never clear them; see
// listDialogs.
func conversation(accountID string, d *tg.Dialog, e entities, now time.Time) (domain.Conversation, bool) {
	c, ok := peerConversation(accountID, d.Peer, e)
	c.Muted = int64(d.NotifySettings.MuteUntil) > now.Unix()

	return c, ok
}

// peerConversation turns a peer into a conversation, for a chat that a
// new message has just started, or reports false when the peer is not
// among the entities.
func peerConversation(accountID string, peer tg.PeerClass, e entities) (domain.Conversation, bool) {
	input, ok := e.peer(peer)
	if !ok {
		return domain.Conversation{}, false
	}

	// A group or channel's own title is always verified: there is no
	// self-chosen personal identity to mark for it, only for a
	// person's own name (see userName), which the PeerUser case below
	// overrides this default for.
	c := domain.Conversation{AccountID: accountID, RemoteID: remoteID(input), Kind: domain.KindGroup, TitleVerified: true}

	switch p := peer.(type) {
	case *tg.PeerUser:
		c.Kind = domain.KindDirect
		c.Title, c.TitleVerified = userName(e.users[p.UserID])
	case *tg.PeerChat:
		c.Title, c.Members = e.chats[p.ChatID].Title, e.chats[p.ChatID].ParticipantsCount
	case *tg.PeerChannel:
		c.Title, c.Members = e.channels[p.ChannelID].Title, e.channels[p.ChannelID].ParticipantsCount
	}

	return c, true
}

// unknownGroupName is what a group or channel conversation is named
// when placeholderConversation has nothing else to go on; its real
// title arrives with the next dialog sync or a message that does
// carry the chat's entity.
const unknownGroupName = "Telegram group"

// placeholderConversation builds a conversation for a peer that
// neither the update's own entities nor anything this connector has
// learned before names: a stranger's very first message must still
// create a conversation and be stored rather than vanish, even though
// its title, and, for a user or channel, its access hash are not
// known yet. A dialog sync or a later message that does carry the
// entity corrects the title once Telegram resolves it (see
// Connector.rememberName); nothing here corrects a conversation
// already created this way, so the one accepted risk is a second,
// fully-resolved conversation for the same peer turning up later -
// preferred over losing the message outright. It reports false only
// for a peer type this connector does not otherwise recognise.
func placeholderConversation(accountID string, peer tg.PeerClass) (domain.Conversation, bool) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindDirect, Title: unknownUserName, TitleVerified: true,
			RemoteID: remoteID(&tg.InputPeerUser{UserID: p.UserID}),
		}, true
	case *tg.PeerChat:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindGroup, Title: unknownGroupName, TitleVerified: true,
			RemoteID: remoteID(&tg.InputPeerChat{ChatID: p.ChatID}),
		}, true
	case *tg.PeerChannel:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindGroup, Title: unknownGroupName, TitleVerified: true,
			RemoteID: remoteID(&tg.InputPeerChannel{ChannelID: p.ChannelID}),
		}, true
	default:
		return domain.Conversation{}, false
	}
}

// placeholderRemoteID is the remote id placeholderConversation would
// have given peer: the same peer, with a user's or channel's access
// hash forced to 0. Sync calls this for every dialog it lists so a
// stale placeholder conversation, created before Telegram's hash for
// that peer was known - whether moments ago or in a run before this
// fix shipped - is found and upgraded (see sync.go). A chat has no
// placeholder shape of its own to upgrade from: placeholderConversation
// gives it the exact same id ("chat:<id>") a full resolution does,
// so there is nothing for this to ever tell apart.
func placeholderRemoteID(peer tg.PeerClass) (string, bool) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return remoteID(&tg.InputPeerUser{UserID: p.UserID}), true
	case *tg.PeerChannel:
		return remoteID(&tg.InputPeerChannel{ChannelID: p.ChannelID}), true
	default:
		return "", false
	}
}

// message turns a Telegram message into ours. Outgoing messages from
// history count as sent; whether they were read is not known here.
// selfID is this account's own user id, for flagging a message that
// mentions it (see mentions.go); it is 0 before Run has signed in, which
// never mentions anyone.
func message(m *tg.Message, e entities, selfID int64) domain.Message {
	mentions, mentionsMe := mentionsFromEntities(m.Entities, e, selfID)
	out := domain.Message{
		RemoteID:   strconv.Itoa(m.ID),
		Text:       messageText(m),
		Outgoing:   m.Out,
		Status:     domain.StatusReceived,
		Created:    int64(m.Date) * 1000,
		Media:      media(m.Media),
		ReplyTo:    replyTo(m.ReplyTo),
		Reactions:  reactions(m.Reactions),
		Mentions:   mentions,
		MentionsMe: mentionsMe,
		Formatting: spansFromEntities(m.Entities, m.Message),
	}

	if m.Out {
		out.SenderID, out.SenderName, out.SenderVerified, out.Status = "self", "You", true, domain.StatusSent
		return out
	}

	// In a direct chat the sender is the chat's other person.
	sender := m.FromID
	if sender == nil {
		sender = m.PeerID
	}

	if u, ok := sender.(*tg.PeerUser); ok {
		out.SenderID = strconv.FormatInt(u.UserID, 10)
		out.SenderName, out.SenderVerified = userName(e.users[u.UserID])
	}

	return out
}

// replyTo reports the message a Telegram message answers, from its
// reply header, or nil when it answers nothing. The header carries only
// the quoted message's id; the store fills in its sender and excerpt
// from its own copy of that message, if it has one.
func replyTo(h tg.MessageReplyHeaderClass) *domain.Reply {
	header, ok := h.(*tg.MessageReplyHeader)
	if !ok {
		return nil
	}

	id, ok := header.GetReplyToMsgID()
	if !ok {
		return nil
	}

	return &domain.Reply{RemoteID: strconv.Itoa(id)}
}

// messageText is a message's text, or a label for media without a
// caption, since every stored message has text.
func messageText(m *tg.Message) string {
	if strings.TrimSpace(m.Message) != "" {
		return m.Message
	}

	switch media := m.Media.(type) {
	case *tg.MessageMediaPhoto:
		return "[Photo]"
	case *tg.MessageMediaDocument:
		return documentLabel(media.Document)
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive, *tg.MessageMediaVenue:
		return "[Location]"
	case *tg.MessageMediaContact:
		return "[Contact]"
	case *tg.MessageMediaPoll:
		return pollPlaceholder(media.Poll.Question.Text)
	default:
		return "[Message]"
	}
}

// reactions turns Telegram's reaction counts into ours, skipping custom
// emoji reactions: Telegram shows the actual custom sticker for those,
// which this UI has no way to draw, so a message with only custom
// reactions is reported with none rather than a misleading placeholder.
func reactions(mr tg.MessageReactions) []domain.Reaction {
	var out []domain.Reaction
	for _, rc := range mr.Results {
		emoji, ok := rc.Reaction.(*tg.ReactionEmoji)
		if !ok {
			continue
		}

		_, mine := rc.GetChosenOrder()
		out = append(out, domain.Reaction{Emoji: emoji.Emoticon, Count: rc.Count, Mine: mine})
	}

	return out
}

// unknownUserName is what a message or conversation is named when
// Telegram gave this response no usable information about who it
// belongs to: neither a resolved user nor a name learned earlier (see
// Connector.resolvedMessage) covers it. Every private chat message
// Telegram delivers live carries no user entity at all, relying on the
// client already knowing the sender from an earlier, fuller response,
// so this is reached only when nothing has taught the connector that
// sender's name yet.
const unknownUserName = "Telegram user"

// userName names a user the way Telegram's chat list does, and
// reports whether that name is verified: the chat with yourself is
// your saved messages, a fixed, always-verified label, never a
// self-chosen claim; otherwise see ownName.
func userName(u *tg.User) (name string, verified bool) {
	if u != nil && u.Self {
		return "Saved Messages", true
	}

	return ownName(u)
}

// ownName names a person by first and last name, then username, and
// reports whether that name is verified: true when u is someone the
// account has saved as a contact or Telegram itself verifies
// (u.Verified - its own badge for notable accounts), false for a bare
// first/last name or username, exactly as self-chosen and unverified
// as a WhatsApp push name (see domain.Message.SenderVerified). The
// generic label this falls back to when u is nil or names neither is
// never marked unverified: it is this connector's own placeholder, not
// a claim the person made.
func ownName(u *tg.User) (name string, verified bool) {
	switch {
	case u == nil:
		return unknownUserName, true
	case strings.TrimSpace(u.FirstName+" "+u.LastName) != "":
		return strings.TrimSpace(u.FirstName + " " + u.LastName), u.Contact || u.Verified
	case u.Username != "":
		return "@" + u.Username, u.Contact || u.Verified
	default:
		return unknownUserName, true
	}
}

// contact turns a user into a contact of the account.
func contact(accountID string, u *tg.User) domain.Contact {
	name, _ := userName(u)

	return domain.Contact{
		AccountID: accountID,
		RemoteID:  remoteID(&tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}),
		Name:      name,
	}
}
