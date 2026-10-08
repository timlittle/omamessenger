package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// messageColumns lists the columns scanMessage reads, in order.
const messageColumns = `id,conversation_id,remote_id,sender_id,sender_name,text,outgoing,status,created,media,edited,reply_to,reactions,mentions,mentions_me,retry_at,retry_attempts,retry_since,attachment_original_path,attachment_original_modtime`

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
	return s.addMessage(ctx, m, true)
}

// AddHistoryMessage stores a message the same way AddMessage does, but
// never counts it towards the conversation's unread total. Older history
// is not new: the service's own unread count, synced separately through
// Unread, is authoritative, so a backfilled message (the first open of a
// conversation whose history was not yet paged locally, or scrolling
// further back) must never raise or lower it.
func (s *Store) AddHistoryMessage(ctx context.Context, m domain.Message) (_ domain.Message, inserted bool, _ error) {
	return s.addMessage(ctx, m, false)
}

// addMessage is AddMessage and AddHistoryMessage's shared implementation.
// countUnread decides whether an unread, incoming message bumps its
// conversation's unread count.
func (s *Store) addMessage(ctx context.Context, m domain.Message, countUnread bool) (_ domain.Message, inserted bool, _ error) {
	if m.ConversationID == "" || m.Text == "" {
		return m, false, fmt.Errorf("store: add message: %w", ErrInvalidMessage)
	}

	if existing, found, err := s.existingByRemote(ctx, m); found || err != nil {
		if err != nil || existing.Media != nil || m.Media == nil {
			return existing, false, err
		}

		return s.fillMedia(ctx, existing, m.Media)
	}

	m = withMessageDefaults(m)
	m = s.resolveReply(ctx, m)
	if err := s.insertMessage(ctx, m, countUnread); err != nil {
		return m, false, wrap("add message", err)
	}

	return m, true, nil
}

// resolveReply fills a reply's quoted sender name and excerpt from the
// message it answers, stored earlier in the same conversation, when the
// caller did not already supply them: a connector reporting an incoming
// reply often knows only the quoted message's remote id, while an
// outgoing reply the app builds already carries both. A quoted message
// not stored here yet, such as one still outside the loaded history,
// leaves the reply as given.
func (s *Store) resolveReply(ctx context.Context, m domain.Message) domain.Message {
	if m.ReplyTo == nil || m.ReplyTo.RemoteID == "" || (m.ReplyTo.SenderName != "" && m.ReplyTo.Text != "") {
		return m
	}

	quoted, err := s.MessageByRemote(ctx, m.ConversationID, m.ReplyTo.RemoteID)
	if err != nil {
		return m
	}

	reply := *m.ReplyTo
	reply.SenderName = quoted.SenderName
	reply.Text = domain.Excerpt(quoted.Text)
	m.ReplyTo = &reply

	return m
}

// fillMedia adds media to a message stored before the service reported
// it, such as one synced before link previews existed.
func (s *Store) fillMedia(ctx context.Context, m domain.Message, media *domain.Media) (domain.Message, bool, error) {
	encoded, err := encodeMedia(media)
	if err != nil {
		return m, false, wrap("fill media", err)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE messages SET media=? WHERE id=?`, encoded, m.ID); err != nil {
		return m, false, wrap("fill media", err)
	}

	m.Media = media

	return m, false, nil
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
// countUnread is passed straight through to bumpConversation.
func (s *Store) insertMessage(ctx context.Context, m domain.Message, countUnread bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if err := bumpConversation(ctx, tx, m, countUnread); err != nil {
		return err
	}

	media, err := encodeMedia(m.Media)
	if err != nil {
		return err
	}

	replyTo, err := encodeReply(m.ReplyTo)
	if err != nil {
		return err
	}

	reactions, err := encodeReactions(m.Reactions)
	if err != nil {
		return err
	}

	mentions, err := encodeMentions(m.Mentions)
	if err != nil {
		return err
	}

	originalPath, originalModTime := originalAttachment(m.Media)

	_, err = tx.ExecContext(ctx, `INSERT INTO messages(`+messageColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.RemoteID, m.SenderID, m.SenderName, m.Text, m.Outgoing, m.Status, m.Created, media, m.Edited, replyTo, reactions, mentions, m.MentionsMe,
		m.RetryAt, m.RetryAttempts, m.RetrySince, originalPath, originalModTime)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// originalAttachment reports media's original file path and modification
// time, or "" and 0 when media is nil or has no original to remember.
func originalAttachment(media *domain.Media) (path string, modTime int64) {
	if media == nil {
		return "", 0
	}

	return media.OriginalPath, media.OriginalModTime
}

// bumpConversation counts m as unread when countUnread is set and it is
// incoming and not yet read, and makes it the preview when it is the
// newest message. Older history arriving later leaves the preview alone.
func bumpConversation(ctx context.Context, tx *sql.Tx, m domain.Message, countUnread bool) error {
	unread := boolInt(countUnread && !m.Outgoing && m.Status != domain.StatusRead)
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

// MessageEdit is what EditMessage applies to a stored message: its new
// text, media, reactions and mentions, bundled into one argument to
// keep the function's signature short.
type MessageEdit struct {
	Text       string
	Media      *domain.Media
	Reactions  []domain.Reaction
	Mentions   []domain.Mention
	MentionsMe bool
}

// EditMessage updates a stored message's text, media and reactions after
// the service reports it changed, found by its conversation and the
// service's id for it. A message that is not stored is ignored, not an
// error.
func (s *Store) EditMessage(ctx context.Context, conversationID, remoteID string, edit MessageEdit) (_ domain.Message, found bool, _ error) {
	m, err := s.MessageByRemote(ctx, conversationID, remoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Message{}, false, nil
	}
	if err != nil {
		return m, false, err
	}

	encodedMedia, err := encodeMedia(edit.Media)
	if err != nil {
		return m, false, wrap("edit message", err)
	}

	encodedReactions, err := encodeReactions(edit.Reactions)
	if err != nil {
		return m, false, wrap("edit message", err)
	}

	encodedMentions, err := encodeMentions(edit.Mentions)
	if err != nil {
		return m, false, wrap("edit message", err)
	}

	_, err = s.db.ExecContext(ctx, `UPDATE messages SET text=?,media=?,edited=1,reactions=?,mentions=?,mentions_me=? WHERE id=?`,
		edit.Text, encodedMedia, encodedReactions, encodedMentions, edit.MentionsMe, m.ID)
	if err != nil {
		return m, false, wrap("edit message", err)
	}

	m.Text, m.Media, m.Edited, m.Reactions = edit.Text, edit.Media, true, edit.Reactions
	m.Mentions, m.MentionsMe = edit.Mentions, edit.MentionsMe

	return m, true, nil
}

// DeleteMessages removes messages a service reports deleted, found by the
// ids it gave them, within conversationRemoteIDs: the conversations of
// the account the ids might belong to. The caller names them explicitly
// because the store has no notion of a service's own rules for when an id
// is unique to one conversation or shared account-wide; a remote id that
// is not stored in one of them is ignored. Each conversation a removal
// touches has its preview and activity fall back to its newest remaining
// message, and its unread count drops by one for every removed message
// that was unread. deleted lists the removed messages, each still
// carrying its conversation id.
func (s *Store) DeleteMessages(ctx context.Context, accountID string, conversationRemoteIDs, remoteIDs []string) ([]domain.Message, error) {
	if len(remoteIDs) == 0 {
		return nil, nil
	}

	convIDs, err := s.deletionScope(ctx, accountID, conversationRemoteIDs)
	if err != nil || len(convIDs) == 0 {
		return nil, err
	}

	deleted, err := s.findDeletable(ctx, convIDs, remoteIDs)
	if err != nil || len(deleted) == 0 {
		return nil, err
	}

	if err := s.removeMessages(ctx, deleted); err != nil {
		return nil, wrap("delete messages", err)
	}

	return deleted, nil
}

// deletionScope resolves each of conversationRemoteIDs to its local
// conversation id, skipping any that is not a conversation of the
// account.
func (s *Store) deletionScope(ctx context.Context, accountID string, conversationRemoteIDs []string) ([]string, error) {
	ids := make([]string, 0, len(conversationRemoteIDs))
	for _, remote := range conversationRemoteIDs {
		conv, err := s.ConversationByRemote(ctx, accountID, remote)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}

		ids = append(ids, conv.ID)
	}

	return ids, nil
}

// findDeletable returns the stored messages among remoteIDs, in any of
// convIDs.
func (s *Store) findDeletable(ctx context.Context, convIDs, remoteIDs []string) ([]domain.Message, error) {
	query := `SELECT ` + messageColumns + ` FROM messages
		WHERE remote_id<>'' AND conversation_id IN (` + placeholders(len(convIDs)) + `)
		AND remote_id IN (` + placeholders(len(remoteIDs)) + `)`

	args := make([]any, 0, len(convIDs)+len(remoteIDs))
	for _, id := range convIDs {
		args = append(args, id)
	}
	for _, id := range remoteIDs {
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrap("find deletable", err)
	}

	messages, err := scanAll(rows, scanMessage)

	return messages, wrap("find deletable", err)
}

// removeMessages deletes messages and brings each conversation they left
// up to date: preview, activity and unread count.
func (s *Store) removeMessages(ctx context.Context, messages []domain.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	touched := map[string]bool{}
	for _, m := range messages {
		if err := deleteOneMessage(ctx, tx, m); err != nil {
			return err
		}
		touched[m.ConversationID] = true
	}

	for conv := range touched {
		if err := refreshAfterDeletion(ctx, tx, conv); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// deleteOneMessage removes a message and lowers its conversation's unread
// count when the message was counted in it.
func deleteOneMessage(ctx context.Context, tx *sql.Tx, m domain.Message) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id=?`, m.ID); err != nil {
		return err
	}

	if m.Outgoing || m.Status == domain.StatusRead {
		return nil
	}

	_, err := tx.ExecContext(ctx, `UPDATE conversations SET unread=MAX(unread-1,0) WHERE id=?`, m.ConversationID)

	return err
}

// refreshAfterDeletion recomputes a conversation's preview and activity
// from whatever messages it has left, after one or more were removed.
func refreshAfterDeletion(ctx context.Context, tx *sql.Tx, conversationID string) error {
	const newest = `SELECT %s FROM messages WHERE conversation_id=? ORDER BY created DESC, rowid DESC LIMIT 1`
	_, err := tx.ExecContext(ctx, `UPDATE conversations SET
			preview=COALESCE((`+fmt.Sprintf(newest, "text")+`),''),
			preview_sender=COALESCE((`+fmt.Sprintf(newest, "sender_name")+`),''),
			preview_out=COALESCE((`+fmt.Sprintf(newest, "outgoing")+`),0),
			last_activity=COALESCE((`+fmt.Sprintf(newest, "created")+`),0)
		WHERE id=?`,
		conversationID, conversationID, conversationID, conversationID, conversationID)

	return err
}

// placeholders returns n comma-separated "?" placeholders for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// OldestRemoteID returns the service's id for the conversation's oldest
// message that has one, or "" when none does. A connector fetches older
// history from there.
func (s *Store) OldestRemoteID(ctx context.Context, conversationID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT remote_id FROM messages
		WHERE conversation_id=? AND remote_id<>'' ORDER BY created, rowid LIMIT 1`, conversationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	return id, wrap("oldest remote id", err)
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
	var media, replyTo, reactions, mentions, originalPath string
	var originalModTime int64
	if err := row.Scan(&m.ID, &m.ConversationID, &m.RemoteID, &m.SenderID, &m.SenderName,
		&m.Text, &m.Outgoing, &m.Status, &m.Created, &media, &m.Edited, &replyTo, &reactions,
		&mentions, &m.MentionsMe, &m.RetryAt, &m.RetryAttempts, &m.RetrySince,
		&originalPath, &originalModTime); err != nil {
		return m, err
	}

	if media != "" {
		m.Media = &domain.Media{}
		if err := json.Unmarshal([]byte(media), m.Media); err != nil {
			return m, err
		}
		m.Media.OriginalPath, m.Media.OriginalModTime = originalPath, originalModTime
	}

	if replyTo != "" {
		m.ReplyTo = &domain.Reply{}
		if err := json.Unmarshal([]byte(replyTo), m.ReplyTo); err != nil {
			return m, err
		}
	}

	if reactions != "" {
		if err := json.Unmarshal([]byte(reactions), &m.Reactions); err != nil {
			return m, err
		}
	}

	var err error
	m.Mentions, err = decodeMentions(mentions)

	return m, err
}

// encodeMedia stores media as JSON, or "" for none.
func encodeMedia(media *domain.Media) (string, error) {
	if media == nil {
		return "", nil
	}

	b, err := json.Marshal(media)

	return string(b), err
}

// encodeReply stores a reply as JSON, or "" for none.
func encodeReply(reply *domain.Reply) (string, error) {
	if reply == nil {
		return "", nil
	}

	b, err := json.Marshal(reply)

	return string(b), err
}
