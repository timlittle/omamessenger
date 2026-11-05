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
		c.Kind = domain.KindDirect
		c.Title = userName(e.users[p.UserID])
	case *tg.PeerChat:
		c.Title, c.Members = e.chats[p.ChatID].Title, e.chats[p.ChatID].ParticipantsCount
	case *tg.PeerChannel:
		c.Title, c.Members = e.channels[p.ChannelID].Title, e.channels[p.ChannelID].ParticipantsCount
	}

	return c, true
}

// unknownGroupName names a group or channel conversation when
// placeholderConversation has nothing else to go on.
const unknownGroupName = "Telegram group"

// placeholderConversation builds a conversation for a peer with no known
// entity yet, so a stranger's first message is still stored rather than
// lost; the title is corrected once Telegram resolves it. It reports false
// only for an unrecognised peer type.
func placeholderConversation(accountID string, peer tg.PeerClass) (domain.Conversation, bool) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindDirect, Title: unknownUserName,
			RemoteID: remoteID(&tg.InputPeerUser{UserID: p.UserID}),
		}, true
	case *tg.PeerChat:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindGroup, Title: unknownGroupName,
			RemoteID: remoteID(&tg.InputPeerChat{ChatID: p.ChatID}),
		}, true
	case *tg.PeerChannel:
		return domain.Conversation{
			AccountID: accountID, Kind: domain.KindGroup, Title: unknownGroupName,
			RemoteID: remoteID(&tg.InputPeerChannel{ChannelID: p.ChannelID}),
		}, true
	default:
		return domain.Conversation{}, false
	}
}

// placeholderRemoteID is the remote id placeholderConversation would have
// given peer, used by sync to find and upgrade stale placeholders.
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
// never mentions anyone. A self-destructing photo or video is never
// downloaded: its media is dropped and its text instead names what it
// was (see domain.ViewOncePlaceholder), and the message's own TTLPeriod,
// counted from this same call since Telegram always deletes it
// message.date+ttl_period later regardless of whether it was read, is
// carried for the sweeper to act on later.
func message(m *tg.Message, e entities, selfID int64) domain.Message {
	viewOnce, kind := selfDestructKind(m.Media)
	text, mentions, mentionsMe := messageTextAndMentions(m, e, selfID, viewOnce, kind)

	var messageMedia *domain.Media
	if !viewOnce {
		messageMedia = media(m.Media)
	}

	out := domain.Message{
		RemoteID:    strconv.Itoa(m.ID),
		Text:        text,
		Outgoing:    m.Out,
		Status:      domain.StatusReceived,
		Created:     int64(m.Date) * 1000,
		Media:       messageMedia,
		ReplyTo:     replyTo(m.ReplyTo),
		Reactions:   reactions(m.Reactions),
		Mentions:    mentions,
		MentionsMe:  mentionsMe,
		Formatting:  spansFromEntities(m.Entities, m.Message),
		ViewOnce:    viewOnce,
		ExpireAfter: int64(m.TTLPeriod),
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

// messageTextAndMentions is message's own text and @-mentions, or the
// fixed view-once placeholder with no mentions at all once viewOnce is
// true: a self-destructing photo or video this connector never
// downloads carries nothing worth scanning for an @-mention token.
func messageTextAndMentions(m *tg.Message, e entities, selfID int64, viewOnce bool, kind string) (text string, mentions []domain.Mention, mentionsMe bool) {
	if viewOnce {
		return domain.ViewOncePlaceholder(kind), nil, false
	}

	mentions, mentionsMe = mentionsFromEntities(m.Entities, e, selfID)

	return messageText(m), mentions, mentionsMe
}

// selfDestructKind reports whether media is Telegram's own
// self-destructing photo or video (its ttl_seconds set outside a
// secret chat), the closest match to WhatsApp's view-once media, and
// which kind it is for domain.ViewOncePlaceholder's wording. A document
// with no recognised voice or video attribute reads as a video, the
// common case for Telegram's own self-destructing media.
func selfDestructKind(mm tg.MessageMediaClass) (viewOnce bool, kind string) {
	switch m := mm.(type) {
	case *tg.MessageMediaPhoto:
		return m.TTLSeconds > 0, domain.MediaPhoto
	case *tg.MessageMediaDocument:
		if m.TTLSeconds <= 0 {
			return false, ""
		}

		return true, selfDestructDocumentKind(m.Document)
	default:
		return false, ""
	}
}

// selfDestructDocumentKind is selfDestructKind's own case for a
// self-destructing document: a voice note keeps its own wording, and
// every other document reads as a video.
func selfDestructDocumentKind(dc tg.DocumentClass) string {
	doc, ok := dc.(*tg.Document)
	if !ok {
		return domain.MediaVideo
	}

	if describe(doc).voice {
		return domain.MediaVoice
	}

	return domain.MediaVideo
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

// userName names a user the way Telegram's chat list does: the chat
// with yourself is your saved messages, a fixed label; otherwise see
// ownName.
func userName(u *tg.User) string {
	if u != nil && u.Self {
		return "Saved Messages"
	}

	return ownName(u)
}

// ownName names a person by first and last name, then username,
// falling back to a generic label when u is nil or names neither.
func ownName(u *tg.User) string {
	switch {
	case u == nil:
		return unknownUserName
	case strings.TrimSpace(u.FirstName+" "+u.LastName) != "":
		return strings.TrimSpace(u.FirstName + " " + u.LastName)
	case u.Username != "":
		return "@" + u.Username
	default:
		return unknownUserName
	}
}

// contact turns a user into a contact of the account. contact's only
// caller reads Telegram's own contacts.getContacts, so every user it
// names is already a saved contact of this account.
func contact(accountID string, u *tg.User) domain.Contact {
	return domain.Contact{
		AccountID: accountID,
		RemoteID:  remoteID(&tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}),
		Name:      userName(u),
	}
}
