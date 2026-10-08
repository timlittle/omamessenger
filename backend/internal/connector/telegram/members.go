package telegram

// members.go lists a group's current members for the @-mention picker:
// channels.getParticipants for a supergroup or channel, or
// messages.getFullChat for a basic group, which keeps its members on
// the chat itself rather than through a paged participants call.

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// membersLimit bounds one channels.getParticipants call; a group with
// more members than this returns only its first page, which already
// covers every group small enough to @-mention someone in by name.
const membersLimit = 200

// Members lists a group's current members, named from whichever
// response carries them.
func (c *Connector) Members(ctx context.Context, conv domain.Conversation) ([]domain.Member, error) {
	api, _, err := c.session()
	if err != nil {
		return nil, err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return nil, err
	}

	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		return channelMembers(ctx, api, p)
	case *tg.InputPeerChat:
		return chatMembers(ctx, api, p.ChatID)
	default:
		return nil, nil
	}
}

// channelMembers lists a supergroup's or channel's recent participants.
func channelMembers(ctx context.Context, api *tg.Client, p *tg.InputPeerChannel) ([]domain.Member, error) {
	result, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash},
		Filter:  &tg.ChannelParticipantsRecent{}, Limit: membersLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram: members: %w", err)
	}

	participants, ok := result.(*tg.ChannelsChannelParticipants)
	if !ok {
		return nil, nil
	}

	e := newEntities(participants.Users, nil)

	return memberList(e), nil
}

// chatMembers lists a basic group's members from its full chat info.
func chatMembers(ctx context.Context, api *tg.Client, chatID int64) ([]domain.Member, error) {
	result, err := api.MessagesGetFullChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("telegram: members: %w", err)
	}

	e := newEntities(result.Users, nil)

	return memberList(e), nil
}

// memberList names every user entities indexed, in no particular order:
// the response that fed it already limited who is included.
func memberList(e entities) []domain.Member {
	out := make([]domain.Member, 0, len(e.users))
	for id, u := range e.users {
		out = append(out, domain.Member{
			ID: remoteID(&tg.InputPeerUser{UserID: id, AccessHash: u.AccessHash}), Name: userName(u),
		})
	}

	return out
}
