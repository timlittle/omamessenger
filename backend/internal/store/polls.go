package store

import (
	"context"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// SetPoll updates a stored message's poll after the service reports it
// changing, such as a new vote tally, without a full edit, found by its
// conversation and the service's id for it. The update is folded onto
// whatever poll the message already carries with domain.MergePoll, so
// a tallies-only update never loses the question or option text an
// earlier report already filled in. A message that is not stored, or
// one with no media to attach a poll to, is ignored, not an error.
func (s *Store) SetPoll(ctx context.Context, conversationID, remoteID string, poll domain.Poll) (_ domain.Message, found bool, _ error) {
	m, err := s.MessageByRemote(ctx, conversationID, remoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Message{}, false, nil
	}
	if err != nil {
		return m, false, err
	}

	media := m.Media
	if media == nil {
		media = &domain.Media{Kind: domain.MediaPoll}
	}

	merged := domain.MergePoll(media.Poll, poll)
	media.Poll = &merged

	encoded, err := encodeMedia(media)
	if err != nil {
		return m, false, wrap("set poll", err)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE messages SET media=? WHERE id=?`, encoded, m.ID); err != nil {
		return m, false, wrap("set poll", err)
	}

	m.Media = media

	return m, true, nil
}
