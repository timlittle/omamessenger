package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// SetReactions updates a stored message's reaction chips after the
// service reports they changed on their own, without a full edit, found
// by its conversation and the service's id for it. A message that is not
// stored is ignored, not an error.
func (s *Store) SetReactions(ctx context.Context, conversationID, remoteID string, reactions []domain.Reaction) (_ domain.Message, found bool, _ error) {
	m, err := s.MessageByRemote(ctx, conversationID, remoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Message{}, false, nil
	}
	if err != nil {
		return m, false, err
	}

	encoded, err := encodeReactions(reactions)
	if err != nil {
		return m, false, wrap("set reactions", err)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE messages SET reactions=? WHERE id=?`, encoded, m.ID); err != nil {
		return m, false, wrap("set reactions", err)
	}

	m.Reactions = reactions

	return m, true, nil
}

// encodeReactions stores reaction chips as JSON, or "" for none.
func encodeReactions(reactions []domain.Reaction) (string, error) {
	if len(reactions) == 0 {
		return "", nil
	}

	b, err := json.Marshal(reactions)

	return string(b), err
}
