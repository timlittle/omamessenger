package telegram

// mentions.go turns Telegram's mention entities into ours, and back.
// Telegram's own offsets are already in UTF-16 code units, the unit
// domain.Mention uses throughout, so neither direction needs the
// byte-position conversion WhatsApp's connector does.

import (
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mentionsFromEntities reads a message's entities for the users it
// mentions, naming each from e, and reports whether selfID is among
// them.
func mentionsFromEntities(ents []tg.MessageEntityClass, e entities, selfID int64) ([]domain.Mention, bool) {
	var out []domain.Mention
	me := false

	for _, ent := range ents {
		mn, ok := ent.(*tg.MessageEntityMentionName)
		if !ok {
			continue
		}

		user := e.users[mn.UserID]
		if user == nil {
			continue
		}

		out = append(out, domain.Mention{
			UserID: remoteID(&tg.InputPeerUser{UserID: mn.UserID, AccessHash: user.AccessHash}),
			Name:   userName(user), Offset: mn.Offset, Length: mn.Length,
		})
		if mn.UserID == selfID {
			me = true
		}
	}

	return out, me
}

// outgoingEntities builds the message entities an outgoing text's
// mentions need, skipping any whose UserID is not a Telegram user
// remote id this connector recognises rather than failing the whole
// send over one bad mention.
func outgoingEntities(mentions []domain.Mention) []tg.MessageEntityClass {
	if len(mentions) == 0 {
		return nil
	}

	out := make([]tg.MessageEntityClass, 0, len(mentions))
	for _, mn := range mentions {
		user, ok := mentionedUser(mn.UserID)
		if !ok {
			continue
		}

		out = append(out, &tg.InputMessageEntityMentionName{
			Offset: mn.Offset, Length: mn.Length, UserID: user,
		})
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// mentionedUser reads a mention's user remote id back into the input
// user Telegram's send request needs, or reports false for anything
// that is not a user remote id this connector made.
func mentionedUser(remoteID string) (tg.InputUserClass, bool) {
	peer, err := inputPeer(remoteID)
	if err != nil {
		return nil, false
	}

	user, ok := peer.(*tg.InputPeerUser)
	if !ok {
		return nil, false
	}

	return &tg.InputUser{UserID: user.UserID, AccessHash: user.AccessHash}, true
}
