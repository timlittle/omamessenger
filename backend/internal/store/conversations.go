package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// conversationColumns lists the columns scanConversation reads, in order.
// Queries alias conversations as c and join accounts as a.
const conversationColumns = `c.id,c.account_id,a.service,c.remote_id,c.kind,c.title,c.members,
	c.preview,c.preview_sender,c.preview_out,c.unread,c.muted,c.pinned,c.pin_order,c.archived,c.hidden,c.last_activity,c.reminder_at,c.reminder_notified_at,c.blocked,c.notification_sound`

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
// count onto a stored conversation. It never touches pinned, archived or
// hidden: most reports, such as a live message's own conversation, carry
// neither pinned nor archived meaningfully, so only SetOrganized, from a
// dialog sync, may set them; hidden is local to this computer and no
// report ever carries it at all.
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
const conversationsByTitle = `SELECT ` + conversationColumns + `, NULL AS match, NULL AS match_message_id, NULL AS match_sender
	` + conversationFrom + `
	WHERE :query = '' OR c.title LIKE :pattern ESCAPE '\'
	ORDER BY c.pinned DESC, c.pin_order ASC, c.last_activity DESC, c.id`

// conversationsByTitleOrMessage lists conversations whose title matches
// pattern or that have a message matching fts. A title match ranks above
// any message match; among message matches, the most recently matching
// conversation comes first, and bm25 relevance only breaks a tie between
// two matches with the same timestamp. matched finds every message hit;
// hits picks the newest one per conversation, which becomes the ranking
// signal and the Match snippet, MatchMessageID and MatchSender, read
// through an external-content FTS5 table kept in step with messages by
// triggers (see migrate.go). The snippet comes from FTS5's own
// snippet() aux function, bounded to 32 tokens around the match
// (FTS5's own max is 64) rather than a message's whole text - up to
// domain.MaxIncomingTextLength, 64 KiB, with no limit of its own on how
// long a single matched word's surrounding text can be - which would
// otherwise make the command palette's "Messages" row lay out far more
// than a search result ever needs to show.
const conversationsByTitleOrMessage = `WITH matched AS (
		SELECT m.conversation_id AS conversation_id, m.id AS message_id, m.sender_name AS sender_name,
			snippet(messages_fts, 0, '', '', '…', 32) AS snippet, bm25(messages_fts) AS rank, m.created AS created, m.rowid AS rowid
		FROM messages_fts
		JOIN messages m ON m.rowid = messages_fts.rowid
		WHERE messages_fts MATCH :fts
	), hits AS (
		SELECT *, ROW_NUMBER() OVER (
			PARTITION BY conversation_id ORDER BY created DESC, rowid DESC
		) AS rn
		FROM matched
	)
	SELECT ` + conversationColumns + `, hits.snippet AS match, hits.message_id AS match_message_id, hits.sender_name AS match_sender
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

// SetNotificationSound records whether a conversation has its own
// custom notification sound: hasCustom true stores "custom", false
// stores "" (inherit the service's or the global default). The sound's
// own bytes live outside the database, in the helper's sound store,
// keyed by this conversation's id (see cache.Sounds); this column only
// ever says whether one is there to look up.
func (s *Store) SetNotificationSound(ctx context.Context, id string, hasCustom bool) error {
	sound := ""
	if hasCustom {
		sound = "custom"
	}

	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET notification_sound=? WHERE id=?`, sound, id)
	if err != nil {
		return wrap("set notification sound", err)
	}

	return requireRow("set notification sound", res)
}

// pinOrderForNewPin is the pin_order a conversation gets the moment it
// is newly pinned: one past whatever the highest among the other pinned
// conversations already is, so a new pin always joins the bottom of the
// pinned group rather than disturbing anyone else's place in it. The
// CASE guards against reassigning it to a conversation that was already
// pinned, whose own place the caller must leave exactly where the user
// left it.
const pinOrderForNewPin = `CASE WHEN pinned=0 THEN (SELECT COALESCE(MAX(pin_order),0)+1 FROM conversations WHERE pinned=1) ELSE pin_order END`

// setOrganizedPinned and setOrganizedUnpinned are SetOrganized's own two
// branches: pinning assigns pin_order the same way SetPinned does, and
// unpinning always clears it.
const (
	setOrganizedPinned   = `UPDATE conversations SET pinned=?,archived=?,pin_order=` + pinOrderForNewPin + ` WHERE id=? AND (pinned<>? OR archived<>?)`
	setOrganizedUnpinned = `UPDATE conversations SET pinned=?,archived=?,pin_order=0 WHERE id=? AND (pinned<>? OR archived<>?)`
)

// SetPinned pins or unpins a conversation, which decides whether it
// leads the conversation list ahead of everything else, and where among
// the other pinned conversations: unlimited, each such conversation
// keeps its own place until moved (see MovePin). Pinning one already
// pinned leaves its place alone; unpinning always clears it, so a later
// pin starts fresh at the bottom rather than resuming an old place a
// still-pinned conversation may have taken over since.
func (s *Store) SetPinned(ctx context.Context, id string, pinned bool) error {
	query := `UPDATE conversations SET pinned=1, pin_order=` + pinOrderForNewPin + ` WHERE id=?`
	if !pinned {
		query = `UPDATE conversations SET pinned=0, pin_order=0 WHERE id=?`
	}

	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return wrap("set pinned", err)
	}

	return requireRow("set pinned", res)
}

// MovePin moves a pinned conversation one place up or down among the
// other pinned conversations, swapping its pin_order with whichever
// pinned neighbor already sits there. It is a no-op, returning "" for
// neighborID, at either edge of the pinned group: moving up from the
// top, or down from the bottom. id must name a currently pinned
// conversation, or this returns domain.ErrNotFound: moving a
// conversation that is not pinned at all is meaningless, the same way
// WhatsApp's own app has no such gesture for an unpinned chat.
func (s *Store) MovePin(ctx context.Context, id string, up bool) (neighborID string, _ error) {
	ids, err := s.pinnedOrder(ctx)
	if err != nil {
		return "", err
	}

	index := slices.Index(ids, id)
	if index < 0 {
		return "", fmt.Errorf("move pin: %w", domain.ErrNotFound)
	}

	step := 1
	if up {
		step = -1
	}

	neighbor := index + step
	if neighbor < 0 || neighbor >= len(ids) {
		return "", nil
	}

	neighborID = ids[neighbor]

	return neighborID, s.swapPinOrder(ctx, id, neighborID)
}

// pinnedOrder lists pinned conversation ids in the order the list shows
// them: lowest pin_order first, the same tiebreakers conversationsByTitle
// uses for any that ever share one.
func (s *Store) pinnedOrder(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM conversations WHERE pinned=1 ORDER BY pin_order ASC, last_activity DESC, id`)
	if err != nil {
		return nil, wrap("pinned order", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("pinned order", err)
		}
		ids = append(ids, id)
	}

	return ids, wrap("pinned order", rows.Err())
}

// swapPinOrder exchanges a and b's pin_order, reading both and writing
// the swap in one transaction so a concurrent change to either between
// the read and the write can never produce a half-done swap.
func (s *Store) swapPinOrder(ctx context.Context, a, b string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap("swap pin order", err)
	}

	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	aOrder, err := pinOrderOf(ctx, tx, a)
	if err != nil {
		return wrap("swap pin order", err)
	}

	bOrder, err := pinOrderOf(ctx, tx, b)
	if err != nil {
		return wrap("swap pin order", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE conversations SET pin_order = CASE id WHEN ? THEN ? WHEN ? THEN ? END WHERE id IN (?,?)`,
		a, bOrder, b, aOrder, a, b); err != nil {
		return wrap("swap pin order", err)
	}

	return wrap("swap pin order", tx.Commit())
}

// pinOrderOf reads one conversation's current pin_order within tx, for
// swapPinOrder.
func pinOrderOf(ctx context.Context, tx *sql.Tx, id string) (int, error) {
	var order int
	err := tx.QueryRowContext(ctx, `SELECT pin_order FROM conversations WHERE id=?`, id).Scan(&order)

	return order, err
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

// RememberPinnedBeforeArchive copies a conversation's current pinned
// and pin_order into pinned_before_archive and pin_order_before_archive,
// so RestorePinnedFromArchive can give the pin back on unarchive: called
// by Commands.SetArchived right before it clears the local pin to match
// a service whose own archive always unpins too (see
// connector.ArchiveUnpinner and docs/decisions.md), which must happen
// first, since SetPinned(ctx, id, false) immediately after would
// otherwise clear pin_order to 0 before this ever reads it.
func (s *Store) RememberPinnedBeforeArchive(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET pinned_before_archive=pinned, pin_order_before_archive=pin_order WHERE id=?`, id)
	if err != nil {
		return wrap("remember pinned before archive", err)
	}

	return requireRow("remember pinned before archive", res)
}

// RestorePinnedFromArchive gives a conversation back the pin it had the
// moment it was last archived (see RememberPinnedBeforeArchive),
// reporting whether it actually had one to restore so the caller knows
// whether to also ask the service to re-pin it: WhatsApp's own archive
// unpins a chat there too, which a person does not expect to lose for
// good just for archiving and later bringing it back (see
// docs/decisions.md). Both before-archive columns are cleared either
// way, so a later archive always starts this bookkeeping fresh rather
// than reapplying a stale value from further back.
func (s *Store) RestorePinnedFromArchive(ctx context.Context, id string) (restored bool, _ error) {
	var wasPinned bool
	var order int
	err := s.db.QueryRowContext(ctx,
		`SELECT pinned_before_archive, pin_order_before_archive FROM conversations WHERE id=?`, id).Scan(&wasPinned, &order)
	if err != nil {
		return false, wrap("restore pinned from archive", err)
	}

	query := `UPDATE conversations SET pinned_before_archive=0, pin_order_before_archive=0 WHERE id=?`
	args := []any{id}
	if wasPinned {
		query = `UPDATE conversations SET pinned=1, pin_order=?, pinned_before_archive=0, pin_order_before_archive=0 WHERE id=?`
		args = []any{order, id}
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return false, wrap("restore pinned from archive", err)
	}

	return wasPinned, nil
}

// SetHidden hides or unhides a conversation from the standard list. This
// is local to this computer only: nothing here is ever reported to or
// read from the service.
func (s *Store) SetHidden(ctx context.Context, id string, hidden bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET hidden=? WHERE id=?`, boolInt(hidden), id)
	if err != nil {
		return wrap("set hidden", err)
	}

	return requireRow("set hidden", res)
}

// SetBlocked records whether the account has blocked the contact
// behind a direct conversation. Like hidden, this never changes what
// Conversations or Conversation otherwise read from the service: a
// connector that blocks or unblocks the contact with the service is
// asked separately (see app.Commands.BlockContact).
func (s *Store) SetBlocked(ctx context.Context, id string, blocked bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET blocked=? WHERE id=?`, boolInt(blocked), id)
	if err != nil {
		return wrap("set blocked", err)
	}

	return requireRow("set blocked", res)
}

// SetReminder snoozes a conversation until at, in Unix milliseconds, or
// clears its reminder when at is 0. This is local to this computer only:
// like hidden, it is never reported to or read from the service. It
// also clears any earlier MarkReminderNotified record, so a freshly set
// reminder is always eligible to notify, even one set to a due time
// that happens to match a previous reminder's.
func (s *Store) SetReminder(ctx context.Context, id string, at int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET reminder_at=?,reminder_notified_at=0 WHERE id=?`, at, id)
	if err != nil {
		return wrap("set reminder", err)
	}

	return requireRow("set reminder", res)
}

// MarkReminderNotified records that the reminder due at "at" for
// conversation id has already been announced, so a restart does not
// announce it again while it stays due: a reminder is never cleared
// automatically once it fires (see docs/decisions.md).
func (s *Store) MarkReminderNotified(ctx context.Context, id string, at int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE conversations SET reminder_notified_at=? WHERE id=?`, at, id)
	if err != nil {
		return wrap("mark reminder notified", err)
	}

	return requireRow("mark reminder notified", res)
}

// PendingReminders lists every conversation with an active reminder,
// soonest due first, for the reminder scheduler to fire or wait on.
func (s *Store) PendingReminders(ctx context.Context) ([]domain.Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+conversationColumns+conversationFrom+`
		WHERE c.reminder_at>0 ORDER BY c.reminder_at ASC, c.id`)
	if err != nil {
		return nil, wrap("pending reminders", err)
	}

	conversations, err := scanAll(rows, scanConversation)

	return conversations, wrap("pending reminders", err)
}

// SetOrganized sets a conversation's pinned and archived state, as a
// dialog sync reports it. changed is false when both already matched.
// Pinning it for the first time sends it to the bottom of the pinned
// group, the same as SetPinned; a sync that reports a conversation
// still pinned leaves its place exactly where the user left it, since
// nothing but MovePin ever changes a still-pinned conversation's order.
func (s *Store) SetOrganized(ctx context.Context, id string, pinned, archived bool) (bool, error) {
	query := setOrganizedPinned
	if !pinned {
		query = setOrganizedUnpinned
	}

	res, err := s.db.ExecContext(ctx, query, boolInt(pinned), boolInt(archived), id, boolInt(pinned), boolInt(archived))
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

// UnreadByService counts unread messages in conversations that are not
// muted, grouped by the owning account's service. A service with no
// unread messages at all is left out of the map, so the bar tooltip can
// tell "no unread" apart from "this service does not exist yet".
func (s *Store) UnreadByService(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.service,SUM(c.unread) FROM conversations c JOIN accounts a ON a.id=c.account_id
		WHERE c.muted=0 GROUP BY a.service HAVING SUM(c.unread)>0`)
	if err != nil {
		return nil, wrap("unread by service", err)
	}
	defer rows.Close()

	byService := map[string]int{}
	for rows.Next() {
		var service string
		var total int
		if err := rows.Scan(&service, &total); err != nil {
			return nil, wrap("unread by service", err)
		}
		byService[service] = total
	}

	return byService, wrap("unread by service", rows.Err())
}

// UpgradeConversationRemoteID changes a conversation's remote id in
// place, keeping its local id, messages and settings, for a connector
// that resolves a better id for a conversation already stored under
// an older one - such as Telegram learning a placeholder peer's real
// access hash, whether moments after creating it or only on a later
// reconnect's full resync. If a conversation already exists under
// newRemoteID, oldRemoteID's messages are moved into it instead and
// the oldRemoteID conversation is removed, so the two never end up
// duplicated; a message the target already has under the same remote
// id is kept once, under the target, never duplicated. It changes
// nothing and reports domain.ErrNotFound when no conversation has
// oldRemoteID - the common, harmless case for a connector that checks
// a peer which never actually needed a placeholder. The format of
// either id is not this package's concern: it only ever compares and
// moves between two opaque strings a connector gives it.
func (s *Store) UpgradeConversationRemoteID(ctx context.Context, accountID, oldRemoteID, newRemoteID string) (domain.Conversation, error) {
	if oldRemoteID == newRemoteID {
		return s.ConversationByRemote(ctx, accountID, oldRemoteID)
	}

	old, err := s.ConversationByRemote(ctx, accountID, oldRemoteID)
	if err != nil {
		return domain.Conversation{}, err
	}

	target, err := s.ConversationByRemote(ctx, accountID, newRemoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return s.renameConversationRemoteID(ctx, old.ID, newRemoteID)
	}
	if err != nil {
		return domain.Conversation{}, err
	}

	return s.mergeConversations(ctx, old.ID, target.ID)
}

// renameConversationRemoteID points a conversation at a new remote id,
// for UpgradeConversationRemoteID's simple case: nothing is stored
// under the new id yet.
func (s *Store) renameConversationRemoteID(ctx context.Context, id, remoteID string) (domain.Conversation, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE conversations SET remote_id=? WHERE id=?`, remoteID, id); err != nil {
		return domain.Conversation{}, wrap("rename conversation remote id", err)
	}

	return s.Conversation(ctx, id)
}

// mergeConversations moves fromID's messages into intoID and removes
// fromID, for UpgradeConversationRemoteID's duplicate case. A message
// fromID has under a remote id intoID already has is left behind:
// moving it would violate the unique index on (conversation_id,
// remote_id), so UPDATE OR IGNORE skips just that row, and it is
// discarded along with fromID itself once the delete below cascades
// onto whatever messages the move could not place.
func (s *Store) mergeConversations(ctx context.Context, fromID, intoID string) (domain.Conversation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so its error carries no information.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE OR IGNORE messages SET conversation_id=? WHERE conversation_id=?`, intoID, fromID); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id=?`, fromID); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Conversation{}, wrap("merge conversations", err)
	}

	return s.Conversation(ctx, intoID)
}

// scanConversation reads one row selected with conversationColumns.
func scanConversation(row scanner) (domain.Conversation, error) {
	return scanConversationWith(row)
}

// scanConversationMatch reads conversationColumns followed by the search
// match, matched message id and matched sender columns.
func scanConversationMatch(row scanner) (domain.Conversation, error) {
	var match, messageID, sender sql.NullString
	c, err := scanConversationWith(row, &match, &messageID, &sender)
	c.Match = match.String
	c.MatchMessageID = messageID.String
	c.MatchSender = sender.String

	return c, err
}

// scanConversationWith reads conversationColumns followed by extra columns.
func scanConversationWith(row scanner, extra ...any) (domain.Conversation, error) {
	var c domain.Conversation
	dest := append([]any{
		&c.ID, &c.AccountID, &c.Service, &c.RemoteID, &c.Kind, &c.Title, &c.Members,
		&c.Preview, &c.PreviewSender, &c.PreviewOut, &c.Unread, &c.Muted, &c.Pinned, &c.PinOrder, &c.Archived, &c.Hidden, &c.LastActivity, &c.ReminderAt, &c.ReminderNotifiedAt, &c.Blocked, &c.NotificationSound,
	}, extra...)
	err := row.Scan(dest...)

	return c, err
}
