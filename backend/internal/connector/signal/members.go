package signal

import (
	"context"
	"fmt"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Members lists conv's current members, for the @-mention picker.
// Only a group conversation has members beyond this account and one
// other person; a direct chat's RemoteID is never a group identifier
// (see isGroupChat), so Members reports an empty list for one rather
// than treating it as an error.
func (c *Connector) Members(ctx context.Context, conv domain.Conversation) ([]domain.Member, error) {
	if !isGroupChat(conv.RemoteID) {
		return nil, nil
	}

	dev, _, err := c.session()
	if err != nil {
		return nil, err
	}

	_, members, _, err := dev.groupInfo(ctx, types.GroupIdentifier(conv.RemoteID))
	if err != nil {
		return nil, fmt.Errorf("signal: list members: %w", err)
	}

	out := make([]domain.Member, len(members))
	for i, m := range members {
		out[i] = domain.Member{ID: senderRemoteID(m.ACI), Name: c.resolveSenderName(ctx, m.ACI)}
	}

	return out, nil
}
