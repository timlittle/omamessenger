package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// messageColumns lists the columns scanMessage reads, in order.
const messageColumns = `id,conversation_id,remote_id,sender_id,sender_name,text,outgoing,status,created`

// Page sizes for Messages.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// ErrInvalidMessage reports a message without a conversation or text.
var ErrInvalidMessage = errors.New("message needs conversationId and text")

// AddMessage stores a message and updates its conversation's preview,
// unread count and activity time. A message whose RemoteID is already stored
// in the conversation is returned unchanged with inserted false, so a
// connector replaying history never double-counts unread messages.
func (s *Store) AddMessage(ctx context.Context, m domain.Message) (_ domain.Message, inserted bool, _ error) {
	if m.ConversationID == "" || m.Text == "" {
		return m, false, fmt.Errorf("store: add message: %w", ErrInvalidMessage)
	}

	if existing, found, err := s.existingByRemote(ctx, m); found || err != nil {
		return existing, false, err
	}

	m = withMessageDefaults(m)
	if err := s.insertMessage(ctx, m); err != nil {
		return m, false, wrap("add message", err)
	}

	return m, true, nil
}

// existingByRemote finds a stored copy of m by its remote id. found is false
// when m has no remote id or no copy is stored.
func (s *Store) existingByRemote(ctx context.Context, m domain.Message) (_ domain.Message, found bool, _ error) {
	if m.RemoteID == "" {
		return m, false, nil
	}

	existing, err := s.MessageByRemote(ctx, m.ConversationID, m.RemoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return m, false, nil
	}

	return existing, err == nil, err
}

// withMessageDefaults assigns an id and the initial delivery status.
func withMessageDefaults(m domain.Message) domain.Message {
	if m.ID == "" {
		m.ID = newID("m")
	}

	switch {
	case m.Status != "":
	case m.Outgoing:
		m.Status = domain.StatusPending
	default:
		m.Status = domain.StatusReceived
	}

	return m
}

// insertMessage writes m and updates its conversation in one transaction.
func (s *Store) insertMessage(ctx context.Context, m domain.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if err := bumpConversation(ctx, tx, m); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO messages(`+messageColumns+`) VALUES(?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.RemoteID, m.SenderID, m.SenderName, m.Text, m.Outgoing, m.Status, m.Created)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// bumpConversation counts m as unread when it is incoming and not yet read,
// and makes it the preview when it is the newest message. Older history
// arriving later leaves the preview alone.
func bumpConversation(ctx context.Context, tx *sql.Tx, m domain.Message) error {
	unread := boolInt(!m.Outgoing && m.Status != domain.StatusRead)
	res, err := tx.ExecContext(ctx, `UPDATE conversations SET
			unread=unread+?,
			preview=CASE WHEN ?>=last_activity THEN ? ELSE preview END,
			preview_sender=CASE WHEN ?>=last_activity THEN ? ELSE preview_sender END,
			preview_out=CASE WHEN ?>=last_activity THEN ? ELSE preview_out END,
			last_activity=MAX(last_activity,?)
		WHERE id=?`,
		unread, m.Created, m.Text, m.Created, m.SenderName, m.Created, boolInt(m.Outgoing),
		m.Created, m.ConversationID)
	if err != nil {
		return err
	}

	return requireRow("bump conversation", res)
}

// Message returns one message by its local id.
func (s *Store) Message(ctx context.Context, id string) (domain.Message, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+messageColumns+` FROM messages WHERE id=?`, id)
	m, err := scanMessage(row)

	return m, wrap("message", err)
}

// MessageByRemote returns one message by its service's id.
func (s *Store) MessageByRemote(ctx context.Context, conversationID, remoteID string) (domain.Message, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+messageColumns+` FROM messages
		WHERE conversation_id=? AND remote_id=? AND remote_id!=''`, conversationID, remoteID)
	m, err := scanMessage(row)

	return m, wrap("message by remote", err)
}

// SetMessageRemoteID records the service's id for a sent message.
func (s *Store) SetMessageRemoteID(ctx context.Context, id, remoteID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET remote_id=? WHERE id=?`, remoteID, id)

	return wrap("set message remote id", err)
}

// UpdateMessageStatus applies a delivery state change when it is forward
// progress (see domain.StatusAdvances). changed reports whether it applied.
func (s *Store) UpdateMessageStatus(ctx context.Context, id, status string) (_ domain.Message, changed bool, _ error) {
	m, err := s.Message(ctx, id)
	if err != nil {
		return m, false, err
	}

	if !domain.StatusAdvances(m.Status, status) {
		return m, false, nil
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE messages SET status=? WHERE id=?`, status, id); err != nil {
		return m, false, wrap("update message status", err)
	}

	m.Status = status

	return m, true, nil
}

// Messages returns up to limit messages older than beforeID, or the newest
// when beforeID is empty, oldest first. hasMore reports whether older
// messages remain. A limit outside 1..MaxPageSize means DefaultPageSize.
func (s *Store) Messages(ctx context.Context, conversationID, beforeID string, limit int) (_ []domain.Message, hasMore bool, _ error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	cursor, args, err := s.pageCursor(ctx, conversationID, beforeID)
	if err != nil {
		return nil, false, err
	}

	// Fetch one extra row to learn whether another page exists.
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM messages
		WHERE conversation_id=?`+cursor+` ORDER BY created DESC, rowid DESC LIMIT ?`,
		append(args, limit+1)...)
	if err != nil {
		return nil, false, wrap("messages", err)
	}

	page, err := scanAll(rows, scanMessage)
	if err != nil {
		return nil, false, wrap("messages", err)
	}

	hasMore = len(page) > limit
	page = page[:min(len(page), limit)]
	slices.Reverse(page)

	return page, hasMore, nil
}

// pageCursor returns the SQL condition and arguments that select messages
// older than beforeID, which must belong to the conversation.
func (s *Store) pageCursor(ctx context.Context, conversationID, beforeID string) (string, []any, error) {
	args := []any{conversationID}
	if beforeID == "" {
		return "", args, nil
	}

	var created, rowid int64
	err := s.db.QueryRowContext(ctx, `SELECT created,rowid FROM messages WHERE id=? AND conversation_id=?`,
		beforeID, conversationID).Scan(&created, &rowid)
	if err != nil {
		return "", nil, wrap("page cursor", err)
	}

	return ` AND (created<? OR (created=? AND rowid<?))`, append(args, created, created, rowid), nil
}

// scanMessage reads one row selected with messageColumns.
func scanMessage(row scanner) (domain.Message, error) {
	var m domain.Message
	err := row.Scan(&m.ID, &m.ConversationID, &m.RemoteID, &m.SenderID, &m.SenderName,
		&m.Text, &m.Outgoing, &m.Status, &m.Created)

	return m, err
}
