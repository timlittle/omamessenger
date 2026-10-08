package whatsapp

// members.go lists a group's current participants for the @-mention
// picker, naming each from WhatsApp's own contact store the way an
// incoming message's sender already is (see normalize_message.go's
// senderDisplayName).

import (
	"context"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.MemberLister = (*Connector)(nil)

// Members lists conv's current participants, or nothing for a direct
// chat, which whatsmeow has no participants to list for.
func (c *Connector) Members(ctx context.Context, conv domain.Conversation) ([]domain.Member, error) {
	dev, _, err := c.session()
	if err != nil {
		return nil, err
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return nil, err
	}

	participants, err := dev.groupParticipants(ctx, jid)
	if err != nil {
		return nil, err
	}

	out := make([]domain.Member, 0, len(participants))
	for _, p := range participants {
		out = append(out, domain.Member{ID: remoteID(p.JID), Name: memberName(ctx, dev, p)})
	}

	return out, nil
}

// memberName names a group participant: their resolved contact or push
// name, falling back to the obfuscated display name WhatsApp gives an
// anonymous participant of an announcement group, or the generic label
// any other unresolved sender gets.
func memberName(ctx context.Context, dev device, p types.GroupParticipant) string {
	if name := dev.contactName(ctx, p.JID); name != "" {
		return name
	}
	if p.DisplayName != "" {
		return p.DisplayName
	}

	return genericSenderName
}
