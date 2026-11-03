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
		name, verified := memberName(ctx, dev, p)
		out = append(out, domain.Member{ID: remoteID(p.JID), Name: name, Verified: verified})
	}

	return out, nil
}

// memberName names a group participant, and reports whether that name
// is verified: their resolved contact name (verified) or push name
// (not, the same gap senderDisplayName already closes for a message's
// own sender - see domain.Message.SenderVerified), falling back to the
// obfuscated display name WhatsApp gives an anonymous participant of
// an announcement group, or the generic label any other unresolved
// sender gets - neither a self-chosen claim, so both report verified.
func memberName(ctx context.Context, dev device, p types.GroupParticipant) (string, bool) {
	if name, verified := dev.contactName(ctx, p.JID); name != "" {
		return name, verified
	}
	if p.DisplayName != "" {
		return p.DisplayName, true
	}

	return genericSenderName, true
}
