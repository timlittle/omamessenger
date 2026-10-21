package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Members lists a conversation's current members, for the composer's
// @-mention picker. It returns an empty list, rather than an error, for
// a direct chat or a service that does not list members: a picker with
// nothing to show is a normal outcome, not something the user needs
// told about.
func (c *Commands) Members(ctx context.Context, conversationID string) ([]domain.Member, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	if c.members == nil || conv.Kind != domain.KindGroup {
		return []domain.Member{}, nil
	}

	members, err := c.members.Members(ctx, conv)
	if members == nil {
		members = []domain.Member{}
	}
	// A group's own membership is reported by whichever service owns
	// it, which a hostile or compromised group admin could inflate
	// without bound; cap what reaches the mention picker (see
	// domain.MaxGroupMembers).
	if len(members) > domain.MaxGroupMembers {
		members = members[:domain.MaxGroupMembers]
	}

	return members, err
}
