package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

const conversationColumns = `c.id,c.account_id,a.service,c.remote_id,c.kind,c.title,c.members,c.preview,
	c.preview_sender,c.preview_out,c.unread,c.muted,c.last_activity`

type scanner interface{ Scan(...any) error }

func scanConversation(row scanner, extra ...any) (domain.Conversation, error) {
	var c domain.Conversation
	dest := append([]any{&c.ID, &c.AccountID, &c.Service, &c.RemoteID, &c.Kind, &c.Title, &c.Members,
		&c.Preview, &c.PreviewSender, &c.PreviewOut, &c.Unread, &c.Muted, &c.LastActivity}, extra...)
	err := row.Scan(dest...)
	return c, err
}

// EnsureConversation returns the conversation for (accountID, remoteID),
// creating it when missing. Title, kind and member count follow the remote.
func (s *Store) EnsureConversation(c domain.Conversation) (domain.Conversation, bool, error) {
	if c.AccountID == "" || c.RemoteID == "" || c.Title == "" {
		return c, false, errors.New("conversation needs accountId, remoteId and title")
	}
	if c.Kind == "" {
		c.Kind = domain.KindDirect
	}
	existing, err := s.ConversationByRemote(c.AccountID, c.RemoteID)
	if err == nil {
		_, err = s.db.Exec(`UPDATE conversations SET title=?,kind=?,members=? WHERE id=?`, c.Title, c.Kind, c.Members, existing.ID)
		if err != nil {
			return existing, false, err
		}
		updated, err := s.Conversation(existing.ID)
		return updated, false, err
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return c, false, err
	}
	if c.ID == "" {
		c.ID = newID("c")
	}
	_, err = s.db.Exec(`INSERT INTO conversations(id,account_id,remote_id,kind,title,members,muted,last_activity)
		VALUES(?,?,?,?,?,?,?,?)`, c.ID, c.AccountID, c.RemoteID, c.Kind, c.Title, c.Members, boolInt(c.Muted), c.LastActivity)
	if err != nil {
		return c, false, err
	}
	created, err := s.Conversation(c.ID)
	return created, true, err
}

func (s *Store) Conversation(id string) (domain.Conversation, error) {
	c, err := scanConversation(s.db.QueryRow(`SELECT `+conversationColumns+`
		FROM conversations c JOIN accounts a ON a.id=c.account_id WHERE c.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.ErrNotFound
	}
	return c, err
}

func (s *Store) ConversationByRemote(accountID, remoteID string) (domain.Conversation, error) {
	c, err := scanConversation(s.db.QueryRow(`SELECT `+conversationColumns+`
		FROM conversations c JOIN accounts a ON a.id=c.account_id WHERE c.account_id=? AND c.remote_id=?`, accountID, remoteID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.ErrNotFound
	}
	return c, err
}

// Conversations lists every conversation, newest activity first. A non-empty
// query keeps conversations whose title or any message text contains it, and
// fills Match with the newest matching message.
func (s *Store) Conversations(query string) ([]domain.Conversation, error) {
	query = strings.TrimSpace(query)
	pattern := "%" + escapeLike(query) + "%"
	rows, err := s.db.Query(`SELECT * FROM (
		SELECT `+conversationColumns+`,
			CASE WHEN ?='' THEN NULL ELSE (SELECT m.text FROM messages m
				WHERE m.conversation_id=c.id AND m.text LIKE ? ESCAPE '\'
				ORDER BY m.created DESC, m.rowid DESC LIMIT 1) END AS match
		FROM conversations c JOIN accounts a ON a.id=c.account_id)
		WHERE ?='' OR title LIKE ? ESCAPE '\' OR match IS NOT NULL
		ORDER BY last_activity DESC, id`, query, pattern, query, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Conversation{}
	for rows.Next() {
		var match sql.NullString
		c, err := scanConversation(rows, &match)
		if err != nil {
			return nil, err
		}
		c.Match = match.String
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) MarkRead(id string) (bool, error) {
	res, err := s.db.Exec(`UPDATE conversations SET unread=0 WHERE id=? AND unread>0`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.Conversation(id); err != nil {
			return false, err
		}
	}
	return n > 0, nil
}

func (s *Store) SetMuted(id string, muted bool) error {
	res, err := s.db.Exec(`UPDATE conversations SET muted=? WHERE id=?`, boolInt(muted), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UnreadTotal counts unread messages in conversations that are not muted.
func (s *Store) UnreadTotal() (int, error) {
	var total int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(unread),0) FROM conversations WHERE muted=0`).Scan(&total)
	return total, err
}
