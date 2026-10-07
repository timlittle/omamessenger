package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// conversationColumns lists the columns scanConversation reads, in order.
// Queries alias conversations as c and join accounts as a.
const conversationColumns = `c.id,c.account_id,a.service,c.remote_id,c.kind,c.title,c.members,
	c.preview,c.preview_sender,c.preview_out,c.unread,c.muted,c.pinned,c.archived,c.last_activity`

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
// count onto a stored conversation. It never touches pinned or archived:
// most reports, such as a live message's own conversation, carry neither
// field meaningfully, so only SetOrganized, from a dialog sync, may set
// them.
func (s *Store) refreshConversation(ctx context.Context, id string, c domain.Conversation) (domain.Conversation, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE conversations SET title=?,kind=?,members=? WHERE id=?`,
		c.Title, c.Kind, c.Members, id)
	if err != nil {
		return c, wrap("update conversation", err)
	}

	return s.Conversation(ctx, id)
}

// insertConversation stores a new conversation, assigning an id if needed.
// It always starts unpinned and unarchived; see refreshConversation.
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

// conversationsByTitle lists conversations whose title matches pattern (or
// all of them when query is empty), newest activity first. It is the whole
// of Conversations' search when query has no word characters to look up in
// messages_fts: an empty FTS5 MATCH argument is invalid, so a query such as
// "_" or "%" can only match a title, the way LIKE always has.
const conversationsByTitle = `SELECT ` + conversationColumns + `, NULL AS match
	` + conversationFrom + `
	WHERE :query = '' OR c.title LIKE :pattern ESCAPE '\'
	ORDER BY c.pinned DESC, c.last_activity DESC, c.id`

// conversationsByTitleOrMessage lists conversations whose title matches
// pattern or that have a message matching fts. A title match ranks above
// any message match; among message matches, the most recently matching
// conversation comes first, and bm25 relevance only breaks a tie between
// two matches with the same timestamp. matched finds every message hit;
// hits picks the newest one per conversation, which becomes both the
// ranking signal and the Match snippet, read through an external-content
// FTS5 table kept in step with messages by triggers (see migrate.go).
const conversationsByTitleOrMessage = `WITH matched AS (
		SELECT m.conversation_id AS conversation_id, m.text AS snippet,
			bm25(messages_fts) AS rank, m.created AS created, m.rowid AS rowid
		FROM messages_fts
		JOIN messages m ON m.rowid = messages_fts.rowid
		WHERE messages_fts MATCH :fts
	), hits AS (
		SELECT *, ROW_NUMBER() OVER (
			PARTITION BY conversation_id ORDER BY created DESC, rowid DESC
		) AS rn
		FROM matched
	)
	SELECT ` + conversationColumns + `, hits.snippet AS match
	` + conversationFrom + `
	LEFT JOIN hits ON hits.conversation_id = c.id AND hits.rn = 1
	WHERE c.title LIKE :pattern ESCAPE '\' OR hits.conversation_id IS NOT NULL
	ORDER BY
		CASE WHEN c.title LIKE :pattern ESCAPE '\' THEN 0 ELSE 1 END,
		COALESCE(hits.created, -1) DESC,
		hits.rank,
		c.last_activity DESC, c.id`

// Conversations lists conversations, newest or best match first. A
// non-empty query keeps those whose title contains it, or that have a
// message matching it word by word as a prefix, case and accent
// insensitively; Match holds the newest matching message, or "" when only
// the title matched.
func (s *Store) Conversations(ctx context.Context, query string) ([]domain.Conversation, error) {
	query = strings.TrimSpace(query)
	pattern := likePattern(query)
	fts := ftsQuery(query)

	stmt, args := conversationsByTitle, []any{sql.Named("query", query), sql.Named("pattern", pattern)}
	if fts != "" {
		stmt, args = conversationsByTitleOrMessage, []any{sql.Named("fts", fts), sql.Named("pattern", pattern)}
	}

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, wrap("conversations", err)
	}

	conversations, err := scanAll(rows, scanConversationMatch)

	return conversations, wrap("conversations", err)
}

// queryWord matches a run of letters or digits in any script, the unit
// ftsQuery turns into one FTS5 search term.
var queryWord = regexp.MustCompile(`[\p{L}\p{N}]+`)

// ftsQuery turns free-text input into an FTS5 query that matches a message
// containing every word of query as a prefix, case and accent
// insensitively. Each word is quoted, which FTS5 always treats as a
// literal token rather than syntax, so quotes, "*", "-" and operator
// keywords such as AND or NEAR in the input can never make the query
// invalid or change its meaning. It returns "" when query has no words,
// since an empty MATCH argument is itself invalid.
func ftsQuery(query string) string {
	words := queryWord.FindAllString(query, -1)
	if len(words) == 0 {
		return ""
	}

	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + w + `"*`
	}

	return strings.Join(terms, " ")
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

// SetPinned pins or unpins a conversation, which decides whether it leads
// the conversation list ahead of everything else.
func (s *Store) SetPinned(ctx context.Context, id string, pinned bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET pinned=? WHERE id=?`, boolInt(pinned), id)
	if err != nil {
		return wrap("set pinned", err)
	}

	return requireRow("set pinned", res)
}

// SetArchived archives or unarchives a conversation, which decides whether
// it shows in the conversation list by default.
func (s *Store) SetArchived(ctx context.Context, id string, archived bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET archived=? WHERE id=?`, boolInt(archived), id)
	if err != nil {
		return wrap("set archived", err)
	}

	return requireRow("set archived", res)
}

// SetOrganized sets a conversation's pinned and archived state, as a
// dialog sync reports it. changed is false when both already matched.
func (s *Store) SetOrganized(ctx context.Context, id string, pinned, archived bool) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET pinned=?,archived=? WHERE id=? AND (pinned<>? OR archived<>?)`,
		boolInt(pinned), boolInt(archived), id, boolInt(pinned), boolInt(archived))
	if err != nil {
		return false, wrap("set organized", err)
	}

	changed, err := rowsChanged("set organized", res)
	if changed || err != nil {
		return changed, err
	}

	// No row changed: either it already matched or it does not exist.
	_, err = s.Conversation(ctx, id)

	return false, err
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
		&c.Preview, &c.PreviewSender, &c.PreviewOut, &c.Unread, &c.Muted, &c.Pinned, &c.Archived, &c.LastActivity,
	}, extra...)
	err := row.Scan(dest...)

	return c, err
}
