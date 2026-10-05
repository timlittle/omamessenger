package store

import (
	"database/sql"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

const messageColumns = `id,conversation_id,remote_id,sender_id,sender_name,text,outgoing,status,created`

func scanMessage(row scanner) (domain.Message, error) {
	var m domain.Message
	err := row.Scan(&m.ID, &m.ConversationID, &m.RemoteID, &m.SenderID, &m.SenderName, &m.Text, &m.Outgoing, &m.Status, &m.Created)
	return m, err
}

// AddMessage stores a message and updates its conversation's preview, unread
// count and activity time. A message whose RemoteID is already stored in the
// conversation is returned unchanged with inserted=false, so replays of
// history from a connector never double-count unread messages.
func (s *Store) AddMessage(m domain.Message) (domain.Message, bool, error) {
	if m.ConversationID == "" || m.Text == "" {
		return m, false, errors.New("message needs conversationId and text")
	}
	if existing, found, err := s.existingByRemote(m); found || err != nil {
		return existing, false, err
	}
	m = withMessageDefaults(m)
	tx, err := s.db.Begin()
	if err != nil {
		return m, false, err
	}
	defer tx.Rollback()
	if err := bumpConversation(tx, m); err != nil {
		return m, false, err
	}
	if _, err := tx.Exec(`INSERT INTO messages(`+messageColumns+`) VALUES(?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.RemoteID, m.SenderID, m.SenderName, m.Text, m.Outgoing, m.Status, m.Created); err != nil {
		return m, false, err
	}
	return m, true, tx.Commit()
}

// existingByRemote finds a stored copy of m by its remote id. found is false
// when m has no remote id or none is stored; err reports lookup failures.
func (s *Store) existingByRemote(m domain.Message) (domain.Message, bool, error) {
	if m.RemoteID == "" {
		return m, false, nil
	}
	existing, err := s.MessageByRemote(m.ConversationID, m.RemoteID)
	switch {
	case err == nil:
		return existing, true, nil
	case errors.Is(err, domain.ErrNotFound):
		return m, false, nil
	default:
		return m, false, err
	}
}

// withMessageDefaults assigns an id and the initial delivery status.
func withMessageDefaults(m domain.Message) domain.Message {
	if m.ID == "" {
		m.ID = newID("m")
	}
	if m.Status == "" && m.Outgoing {
		m.Status = domain.StatusPending
	}
	if m.Status == "" {
		m.Status = domain.StatusReceived
	}
	return m
}

// bumpConversation counts m as unread when it is incoming and not yet read,
// and makes it the preview when it is the newest message.
func bumpConversation(tx *sql.Tx, m domain.Message) error {
	res, err := tx.Exec(`UPDATE conversations SET
			unread=unread+?,
			preview=CASE WHEN ?>=last_activity THEN ? ELSE preview END,
			preview_sender=CASE WHEN ?>=last_activity THEN ? ELSE preview_sender END,
			preview_out=CASE WHEN ?>=last_activity THEN ? ELSE preview_out END,
			last_activity=MAX(last_activity,?)
		WHERE id=?`,
		boolInt(!m.Outgoing && m.Status != domain.StatusRead),
		m.Created, m.Text, m.Created, m.SenderName, m.Created, boolInt(m.Outgoing), m.Created, m.ConversationID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) Message(id string) (domain.Message, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+messageColumns+` FROM messages WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return m, domain.ErrNotFound
	}
	return m, err
}

func (s *Store) MessageByRemote(conversationID, remoteID string) (domain.Message, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+messageColumns+` FROM messages
		WHERE conversation_id=? AND remote_id=? AND remote_id!=''`, conversationID, remoteID))
	if errors.Is(err, sql.ErrNoRows) {
		return m, domain.ErrNotFound
	}
	return m, err
}

// SetMessageRemoteID records the service's id for a sent message.
func (s *Store) SetMessageRemoteID(id, remoteID string) error {
	_, err := s.db.Exec(`UPDATE messages SET remote_id=? WHERE id=?`, remoteID, id)
	return err
}

// UpdateMessageStatus applies a delivery state change when it is forward
// progress (see domain.StatusAdvances). changed reports whether it applied.
func (s *Store) UpdateMessageStatus(id, status string) (domain.Message, bool, error) {
	m, err := s.Message(id)
	if err != nil {
		return m, false, err
	}
	if !domain.StatusAdvances(m.Status, status) {
		return m, false, nil
	}
	if _, err := s.db.Exec(`UPDATE messages SET status=? WHERE id=?`, status, id); err != nil {
		return m, false, err
	}
	m.Status = status
	return m, true, nil
}

// Messages returns up to limit messages older than beforeID (or the newest
// when beforeID is empty), oldest first, and whether older ones remain.
func (s *Store) Messages(conversationID, beforeID string, limit int) ([]domain.Message, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	cursor, args, err := s.pageCursor(conversationID, beforeID)
	if err != nil {
		return nil, false, err
	}
	rows, err := s.db.Query(`SELECT `+messageColumns+` FROM messages WHERE conversation_id=?`+cursor+`
		ORDER BY created DESC, rowid DESC LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	newestFirst, err := scanMessages(rows)
	if err != nil {
		return nil, false, err
	}
	page, hasMore := oldestFirstPage(newestFirst, limit)
	return page, hasMore, nil
}

// pageCursor returns the SQL condition and arguments selecting messages
// older than beforeID, which must belong to the conversation.
func (s *Store) pageCursor(conversationID, beforeID string) (string, []any, error) {
	args := []any{conversationID}
	if beforeID == "" {
		return "", args, nil
	}
	var created, rowid int64
	err := s.db.QueryRow(`SELECT created,rowid FROM messages WHERE id=? AND conversation_id=?`, beforeID, conversationID).Scan(&created, &rowid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, domain.ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	return ` AND (created<? OR (created=? AND rowid<?))`, append(args, created, created, rowid), nil
}

func scanMessages(rows *sql.Rows) ([]domain.Message, error) {
	out := []domain.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// oldestFirstPage keeps the newest limit messages of a newest-first slice
// fetched with one extra row, reverses them, and reports whether the extra
// row existed.
func oldestFirstPage(newestFirst []domain.Message, limit int) ([]domain.Message, bool) {
	hasMore := len(newestFirst) > limit
	if hasMore {
		newestFirst = newestFirst[:limit]
	}
	for i, j := 0, len(newestFirst)-1; i < j; i, j = i+1, j-1 {
		newestFirst[i], newestFirst[j] = newestFirst[j], newestFirst[i]
	}
	return newestFirst, hasMore
}
