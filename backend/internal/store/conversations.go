package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// conversationColumns lists the columns scanConversation reads, in order.
// Queries alias conversations as c and join accounts as a.
const conversationColumns = `c.id,c.account_id,a.service,c.remote_id,c.kind,c.title,c.members,
	c.preview,c.preview_sender,c.preview_out,c.unread,c.muted,c.last_activity`

// conversationFrom is the FROM clause conversationColumns expects.
const conversationFrom = ` FROM conversations c JOIN accounts a ON a.id=c.account_id`

// ErrInvalidConversation reports a conversation without an account, a
// remote id or a title.
var ErrInvalidConversation = errors.New("conversation needs accountId, remoteId and title")

// EnsureConversation returns the conversation for the account and remote id,
// creating it when missing. Title, kind and member count follow the remote
// service. created reports whether a new conversation was stored.
func (s *Store) EnsureConversation(ctx context.Context, c domain.Conversation) (_ domain.Conversation, created bool, _ error) {
	if c.AccountID == "" || c.RemoteID == "" || c.Title == "" {
		return c, false, fmt.Errorf("store: ensure conversation: %w", ErrInvalidConversation)
	}

	if c.Kind == "" {
		c.Kind = domain.KindDirect
	}

	existing, err := s.ConversationByRemote(ctx, c.AccountID, c.RemoteID)
	if err == nil {
		updated, err := s.refreshConversation(ctx, existing.ID, c)
		return updated, false, err
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return c, false, err
	}

	inserted, err := s.insertConversation(ctx, c)

	return inserted, err == nil, err
}

// refreshConversation copies the remote service's title, kind and member
// count onto a stored conversation.
func (s *Store) refreshConversation(ctx context.Context, id string, c domain.Conversation) (domain.Conversation, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE conversations SET title=?,kind=?,members=? WHERE id=?`,
		c.Title, c.Kind, c.Members, id)
	if err != nil {
		return c, wrap("update conversation", err)
	}

	return s.Conversation(ctx, id)
}

// insertConversation stores a new conversation, assigning an id if needed.
func (s *Store) insertConversation(ctx context.Context, c domain.Conversation) (domain.Conversation, error) {
	if c.ID == "" {
		c.ID = newID("c")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO conversations
		(id,account_id,remote_id,kind,title,members,muted,last_activity) VALUES(?,?,?,?,?,?,?,?)`,
		c.ID, c.AccountID, c.RemoteID, c.Kind, c.Title, c.Members, boolInt(c.Muted), c.LastActivity)
	if err != nil {
		return c, wrap("insert conversation", err)
	}

	return s.Conversation(ctx, c.ID)
}

// Conversation returns one conversation by its local id.
func (s *Store) Conversation(ctx context.Context, id string) (domain.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+conversationColumns+conversationFrom+` WHERE c.id=?`, id)
	c, err := scanConversation(row)

	return c, wrap("conversation", err)
}

// ConversationByRemote returns one conversation by its service's id.
func (s *Store) ConversationByRemote(ctx context.Context, accountID, remoteID string) (domain.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+conversationColumns+conversationFrom+`
		WHERE c.account_id=? AND c.remote_id=?`, accountID, remoteID)
	c, err := scanConversation(row)

	return c, wrap("conversation by remote", err)
}

// Conversations lists conversations, newest activity first. A non-empty
// query keeps those whose title or any message contains it, ignoring case,
// and fills Match with the newest matching message.
func (s *Store) Conversations(ctx context.Context, query string) ([]domain.Conversation, error) {
	query = strings.TrimSpace(query)
	pattern := likePattern(query)
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM (
		SELECT `+conversationColumns+`,
			CASE WHEN ?='' THEN NULL ELSE (SELECT m.text FROM messages m
				WHERE m.conversation_id=c.id AND m.text LIKE ? ESCAPE '\'
				ORDER BY m.created DESC, m.rowid DESC LIMIT 1) END AS match
		`+conversationFrom+`)
		WHERE ?='' OR title LIKE ? ESCAPE '\' OR match IS NOT NULL
		ORDER BY last_activity DESC, id`, query, pattern, query, pattern)
	if err != nil {
		return nil, wrap("conversations", err)
	}

	conversations, err := scanAll(rows, scanConversationMatch)

	return conversations, wrap("conversations", err)
}

// MarkRead clears a conversation's unread count. changed is false when it
// was already read.
func (s *Store) MarkRead(ctx context.Context, id string) (changed bool, _ error) {
	return s.SetUnread(ctx, id, 0)
}

// SetUnread sets a conversation's unread count, as the service counts it.
// changed is false when it already had that count.
func (s *Store) SetUnread(ctx context.Context, id string, count int) (changed bool, _ error) {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET unread=? WHERE id=? AND unread<>?`, count, id, count)
	if err != nil {
		return false, wrap("set unread", err)
	}

	changed, err = rowsChanged("set unread", res)
	if changed || err != nil {
		return changed, err
	}

	// No row changed: either it already had that count or it does not exist.
	_, err = s.Conversation(ctx, id)

	return false, err
}

// SetMuted mutes or unmutes a conversation.
func (s *Store) SetMuted(ctx context.Context, id string, muted bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET muted=? WHERE id=?`, boolInt(muted), id)
	if err != nil {
		return wrap("set muted", err)
	}

	return requireRow("set muted", res)
}

// UnreadTotal counts unread messages in conversations that are not muted.
func (s *Store) UnreadTotal(ctx context.Context) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(unread),0) FROM conversations WHERE muted=0`).Scan(&total)

	return total, wrap("unread total", err)
}

// scanConversation reads one row selected with conversationColumns.
func scanConversation(row scanner) (domain.Conversation, error) {
	return scanConversationWith(row)
}

// scanConversationMatch reads conversationColumns followed by the search
// match column.
func scanConversationMatch(row scanner) (domain.Conversation, error) {
	var match sql.NullString
	c, err := scanConversationWith(row, &match)
	c.Match = match.String

	return c, err
}

// scanConversationWith reads conversationColumns followed by extra columns.
func scanConversationWith(row scanner, extra ...any) (domain.Conversation, error) {
	var c domain.Conversation
	dest := append([]any{
		&c.ID, &c.AccountID, &c.Service, &c.RemoteID, &c.Kind, &c.Title, &c.Members,
		&c.Preview, &c.PreviewSender, &c.PreviewOut, &c.Unread, &c.Muted, &c.LastActivity,
	}, extra...)
	err := row.Scan(dest...)

	return c, err
}
