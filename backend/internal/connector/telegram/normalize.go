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

	c := domain.Conversation{AccountID: accountID, RemoteID: remoteID(input), Kind: domain.KindGroup}

	switch p := peer.(type) {
	case *tg.PeerUser:
		c.Kind, c.Title = domain.KindDirect, userName(e.users[p.UserID])
	case *tg.PeerChat:
		c.Title, c.Members = e.chats[p.ChatID].Title, e.chats[p.ChatID].ParticipantsCount
	case *tg.PeerChannel:
		c.Title, c.Members = e.channels[p.ChannelID].Title, e.channels[p.ChannelID].ParticipantsCount
	}

	return c, true
}

// message turns a Telegram message into ours. Outgoing messages from
// history count as sent; whether they were read is not known here.
func message(m *tg.Message, e entities) domain.Message {
	out := domain.Message{
		RemoteID:  strconv.Itoa(m.ID),
		Text:      messageText(m),
		Outgoing:  m.Out,
		Status:    domain.StatusReceived,
		Created:   int64(m.Date) * 1000,
		Media:     media(m.Media),
		Reactions: reactions(m.Reactions),
	}

	if m.Out {
		out.SenderID, out.SenderName, out.Status = "self", "You", domain.StatusSent
		return out
	}

	// In a direct chat the sender is the chat's other person.
	sender := m.FromID
	if sender == nil {
		sender = m.PeerID
	}

	if u, ok := sender.(*tg.PeerUser); ok {
		out.SenderID = strconv.FormatInt(u.UserID, 10)
		out.SenderName = userName(e.users[u.UserID])
	}

	return out
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
		return "[Poll]"
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

// userName names a user the way Telegram's chat list does: the chat with
// yourself is your saved messages.
func userName(u *tg.User) string {
	if u != nil && u.Self {
		return "Saved Messages"
	}

	return ownName(u)
}

// ownName names a person by first and last name, then username.
func ownName(u *tg.User) string {
	switch {
	case u == nil:
		return "Telegram user"
	case strings.TrimSpace(u.FirstName+" "+u.LastName) != "":
		return strings.TrimSpace(u.FirstName + " " + u.LastName)
	case u.Username != "":
		return "@" + u.Username
	default:
		return "Telegram user"
	}
}

// contact turns a user into a contact of the account.
func contact(accountID string, u *tg.User) domain.Contact {
	return domain.Contact{
		AccountID: accountID,
		RemoteID:  remoteID(&tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}),
		Name:      userName(u),
	}
}
