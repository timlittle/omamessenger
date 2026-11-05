package signal

import (
	"context"
	"fmt"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Members lists conv's current members, for the @-mention picker.
// A direct chat reports an empty list, not an error.
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
