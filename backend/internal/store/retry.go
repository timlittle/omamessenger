package store

// retry.go persists the automatic retry scheduler's own state for a
// failed outgoing message - its next attempt, how many it has made and
// when its failure streak began - and the two lookups the scheduler and
// the outgoing media area's sweep need alongside it: whether a message
// is still stored at all, and how many failed messages still carry an
// attachment.

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ScheduleMessageRetry sets when a failed message's next automatic
// retry attempt is due, how many automatic attempts its current
// failure streak has had, and when that streak began, so the scheduler
// can apply the backoff and its bound even after a restart.
func (s *Store) ScheduleMessageRetry(ctx context.Context, id string, at int64, attempts int, since int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET retry_at=?,retry_attempts=?,retry_since=? WHERE id=?`,
		at, attempts, since, id)

	return wrap("schedule message retry", err)
}

// StopMessageRetry clears a failed message's next scheduled retry,
// leaving it failed with nothing further scheduled: the service refused
// it for a reason retrying will not fix, or its failure streak has run
// for as long as it will. Its attempt count and streak start are left
// as a record of what was tried; ClearMessageRetry is what resets those.
func (s *Store) StopMessageRetry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET retry_at=0 WHERE id=?`, id)

	return wrap("stop message retry", err)
}

// ClearMessageRetry resets a message's retry state entirely: nothing
// scheduled, no attempts counted, no streak start. Call it once a retry
// succeeds, or the user retries it themselves, since a retry asked for
// starts the backoff over.
func (s *Store) ClearMessageRetry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET retry_at=0,retry_attempts=0,retry_since=0 WHERE id=?`, id)

	return wrap("clear message retry", err)
}

// PendingRetries returns every failed outgoing message with an
// automatic retry scheduled, soonest due first, so the scheduler can
// re-arm itself for whichever is next, including right after a restart.
func (s *Store) PendingRetries(ctx context.Context) ([]domain.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM messages
		WHERE outgoing=1 AND status=? AND retry_at>0 ORDER BY retry_at`, domain.StatusFailed)
	if err != nil {
		return nil, wrap("pending retries", err)
	}

	messages, err := scanAll(rows, scanMessage)

	return messages, wrap("pending retries", err)
}

// MessageExists reports whether a message with id is still stored, for
// the outgoing media area's sweep to tell an orphaned copy - one whose
// message no longer exists at all - from a pending or failed message's
// copy, which it must never remove.
func (s *Store) MessageExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id=?)`, id).Scan(&exists)

	return exists, wrap("message exists", err)
}

// FailedAttachmentCount counts failed outgoing messages that still carry
// an attachment, for the doctor's "Outgoing attachments" warning to say
// what is filling the area, not just that it is full.
func (s *Store) FailedAttachmentCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE outgoing=1 AND status=? AND media<>''`,
		domain.StatusFailed).Scan(&n)

	return n, wrap("failed attachment count", err)
}

// DeleteMessagesByID removes messages found by their own local ids,
// for a failed outgoing message that never reached its service and so
// has no remote id for DeleteMessages to match. Each conversation a
// removal touches has its preview, activity and unread count brought up
// to date, the same as DeleteMessages. deleted lists the removed
// messages, each still carrying its conversation id.
func (s *Store) DeleteMessagesByID(ctx context.Context, ids []string) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	messages, err := s.findByIDs(ctx, ids)
	if err != nil || len(messages) == 0 {
		return nil, err
	}

	if err := s.removeMessages(ctx, messages); err != nil {
		return nil, wrap("delete messages by id", err)
	}

	return messages, nil
}

// findByIDs returns the stored messages among ids.
func (s *Store) findByIDs(ctx context.Context, ids []string) ([]domain.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM messages WHERE id IN (`+placeholders(len(ids))+`)`,
		toArgs(ids)...)
	if err != nil {
		return nil, wrap("find by ids", err)
	}

	messages, err := scanAll(rows, scanMessage)

	return messages, wrap("find by ids", err)
}

// toArgs turns ids into a slice of any for a variadic SQL call.
func toArgs(ids []string) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	return args
}
