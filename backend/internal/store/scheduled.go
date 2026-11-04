package store

// scheduled.go persists messages queued to send at a chosen time instead of
// right away; once due, ClaimScheduledMessage removes the row and hands it
// to the normal send path (see docs/decisions.md).

import (
	"context"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// scheduledColumns lists the columns scanScheduled reads, in order.
const scheduledColumns = `id,conversation_id,text,send_at,created`

// AddScheduledMessage queues m to send later, assigning it an id if it
// has none and a created time if it has none, and returns it as
// stored.
func (s *Store) AddScheduledMessage(ctx context.Context, m domain.ScheduledMessage) (domain.ScheduledMessage, error) {
	if m.ID == "" {
		m.ID = newID("sm")
	}
	if m.Created == 0 {
		m.Created = time.Now().UnixMilli()
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO scheduled_messages(`+scheduledColumns+`) VALUES(?,?,?,?,?)`,
		m.ID, m.ConversationID, m.Text, m.SendAt, m.Created)

	return m, wrap("add scheduled message", err)
}

// ScheduledMessage returns one queued message by its id.
func (s *Store) ScheduledMessage(ctx context.Context, id string) (domain.ScheduledMessage, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+scheduledColumns+` FROM scheduled_messages WHERE id=?`, id)
	m, err := scanScheduled(row)

	return m, wrap("scheduled message", err)
}

// ScheduledMessages lists every queued message, soonest due first, for
// the "Scheduled messages" command and the sender that fires or waits
// on whichever is next.
func (s *Store) ScheduledMessages(ctx context.Context) ([]domain.ScheduledMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+scheduledColumns+` FROM scheduled_messages ORDER BY send_at, created`)
	if err != nil {
		return nil, wrap("scheduled messages", err)
	}

	messages, err := scanAll(rows, scanScheduled)

	return messages, wrap("scheduled messages", err)
}

// UpdateScheduledMessage changes a queued message's text and send
// time, failing with domain.ErrNotFound once it has already been
// claimed (sent or sent early) or cancelled.
func (s *Store) UpdateScheduledMessage(ctx context.Context, id, text string, sendAt int64) (domain.ScheduledMessage, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE scheduled_messages SET text=?,send_at=? WHERE id=?`, text, sendAt, id)
	if err != nil {
		return domain.ScheduledMessage{}, wrap("update scheduled message", err)
	}
	if err := requireRow("update scheduled message", res); err != nil {
		return domain.ScheduledMessage{}, err
	}

	return s.ScheduledMessage(ctx, id)
}

// CancelScheduledMessage removes a queued message outright, failing
// with domain.ErrNotFound if it no longer exists.
func (s *Store) CancelScheduledMessage(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_messages WHERE id=?`, id)
	if err != nil {
		return wrap("cancel scheduled message", err)
	}

	return requireRow("cancel scheduled message", res)
}

// ClaimScheduledMessage removes a queued message, reporting whether this
// call is the one that removed it; false means another claim or a
// cancellation already won the race.
func (s *Store) ClaimScheduledMessage(ctx context.Context, id string) (claimed bool, _ error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_messages WHERE id=?`, id)
	if err != nil {
		return false, wrap("claim scheduled message", err)
	}

	return rowsChanged("claim scheduled message", res)
}

// scanScheduled reads one row selected with scheduledColumns.
func scanScheduled(row scanner) (domain.ScheduledMessage, error) {
	var m domain.ScheduledMessage
	err := row.Scan(&m.ID, &m.ConversationID, &m.Text, &m.SendAt, &m.Created)

	return m, err
}
