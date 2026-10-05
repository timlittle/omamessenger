// Package store persists the normalized domain in SQLite.
//
// Timestamps are integer Unix milliseconds. Ordering always breaks ties on
// rowid so two messages in the same millisecond keep their arrival order.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

// Open creates the database (owner-only) and applies pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err != nil {
		return nil, err
	} else {
		f.Close()
	}
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// migrations are applied in order; PRAGMA user_version records progress.
// Never edit a released migration: append a new one.
var migrations = []string{
	`CREATE TABLE accounts(
		id TEXT PRIMARY KEY,
		service TEXT NOT NULL,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'offline',
		detail TEXT NOT NULL DEFAULT '');
	CREATE TABLE contacts(
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL,
		name TEXT NOT NULL,
		PRIMARY KEY(account_id, remote_id));
	CREATE TABLE conversations(
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		members INTEGER NOT NULL DEFAULT 0,
		preview TEXT NOT NULL DEFAULT '',
		preview_sender TEXT NOT NULL DEFAULT '',
		preview_out INTEGER NOT NULL DEFAULT 0,
		unread INTEGER NOT NULL DEFAULT 0,
		muted INTEGER NOT NULL DEFAULT 0,
		last_activity INTEGER NOT NULL DEFAULT 0,
		UNIQUE(account_id, remote_id));
	CREATE TABLE messages(
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		remote_id TEXT NOT NULL DEFAULT '',
		sender_id TEXT NOT NULL DEFAULT '',
		sender_name TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL,
		outgoing INTEGER NOT NULL,
		status TEXT NOT NULL,
		created INTEGER NOT NULL);
	CREATE UNIQUE INDEX messages_remote ON messages(conversation_id, remote_id) WHERE remote_id != '';
	CREATE INDEX messages_timeline ON messages(conversation_id, created);`,
}

// SchemaVersion is the version a freshly opened store reports.
var SchemaVersion = len(migrations)

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema %d is newer than this helper supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// NewID returns a random identifier with a readable prefix.
func NewID(prefix string) string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// escapeLike makes user input literal inside a LIKE pattern using '\' as the
// escape character.
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

// ---------------------------------------------------------------- accounts

func (s *Store) UpsertAccount(a domain.Account) error {
	if a.ID == "" || a.Name == "" || !domain.ValidService(a.Service) {
		return fmt.Errorf("invalid account %q", a.ID)
	}
	if a.Status == "" {
		a.Status = domain.AccountOffline
	}
	_, err := s.db.Exec(`INSERT INTO accounts(id,service,name,status,detail) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET service=excluded.service,name=excluded.name`,
		a.ID, a.Service, a.Name, a.Status, a.Detail)
	return err
}

func (s *Store) SetAccountStatus(id, status, detail string) (domain.Account, error) {
	res, err := s.db.Exec(`UPDATE accounts SET status=?,detail=? WHERE id=?`, status, detail, id)
	if err != nil {
		return domain.Account{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Account{}, ErrNotFound
	}
	return s.Account(id)
}

func (s *Store) Account(id string) (domain.Account, error) {
	var a domain.Account
	err := s.db.QueryRow(`SELECT id,service,name,status,detail FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Service, &a.Name, &a.Status, &a.Detail)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) Accounts() ([]domain.Account, error) {
	rows, err := s.db.Query(`SELECT id,service,name,status,detail FROM accounts ORDER BY service,name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Account{}
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.Service, &a.Name, &a.Status, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- contacts

func (s *Store) UpsertContact(c domain.Contact) error {
	_, err := s.db.Exec(`INSERT INTO contacts(account_id,remote_id,name) VALUES(?,?,?)
		ON CONFLICT(account_id,remote_id) DO UPDATE SET name=excluded.name`, c.AccountID, c.RemoteID, c.Name)
	return err
}

func (s *Store) Contact(accountID, remoteID string) (domain.Contact, error) {
	c := domain.Contact{AccountID: accountID, RemoteID: remoteID}
	err := s.db.QueryRow(`SELECT name FROM contacts WHERE account_id=? AND remote_id=?`, accountID, remoteID).Scan(&c.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) Contacts(accountID, query string) ([]domain.Contact, error) {
	pattern := "%" + escapeLike(strings.TrimSpace(query)) + "%"
	rows, err := s.db.Query(`SELECT account_id,remote_id,name FROM contacts
		WHERE account_id=? AND name LIKE ? ESCAPE '\' ORDER BY name COLLATE NOCASE`, accountID, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Contact{}
	for rows.Next() {
		var c domain.Contact
		if err := rows.Scan(&c.AccountID, &c.RemoteID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- conversations

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
	if !errors.Is(err, ErrNotFound) {
		return c, false, err
	}
	if c.ID == "" {
		c.ID = NewID("c")
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
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) ConversationByRemote(accountID, remoteID string) (domain.Conversation, error) {
	c, err := scanConversation(s.db.QueryRow(`SELECT `+conversationColumns+`
		FROM conversations c JOIN accounts a ON a.id=c.account_id WHERE c.account_id=? AND c.remote_id=?`, accountID, remoteID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
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
		return ErrNotFound
	}
	return nil
}

// UnreadTotal counts unread messages in conversations that are not muted.
func (s *Store) UnreadTotal() (int, error) {
	var total int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(unread),0) FROM conversations WHERE muted=0`).Scan(&total)
	return total, err
}

// ---------------------------------------------------------------- messages

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
	if m.RemoteID != "" {
		if existing, err := s.MessageByRemote(m.ConversationID, m.RemoteID); err == nil {
			return existing, false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return m, false, err
		}
	}
	if m.ID == "" {
		m.ID = NewID("m")
	}
	if m.Status == "" {
		if m.Outgoing {
			m.Status = domain.StatusPending
		} else {
			m.Status = domain.StatusReceived
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return m, false, err
	}
	defer tx.Rollback()
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
		return m, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return m, false, ErrNotFound
	}
	if _, err := tx.Exec(`INSERT INTO messages(`+messageColumns+`) VALUES(?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.RemoteID, m.SenderID, m.SenderName, m.Text, m.Outgoing, m.Status, m.Created); err != nil {
		return m, false, err
	}
	return m, true, tx.Commit()
}

func (s *Store) Message(id string) (domain.Message, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+messageColumns+` FROM messages WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (s *Store) MessageByRemote(conversationID, remoteID string) (domain.Message, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+messageColumns+` FROM messages
		WHERE conversation_id=? AND remote_id=? AND remote_id!=''`, conversationID, remoteID))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
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
	args := []any{conversationID}
	cursor := ""
	if beforeID != "" {
		var created, rowid int64
		err := s.db.QueryRow(`SELECT created,rowid FROM messages WHERE id=? AND conversation_id=?`, beforeID, conversationID).Scan(&created, &rowid)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, ErrNotFound
		}
		if err != nil {
			return nil, false, err
		}
		cursor = ` AND (created<? OR (created=? AND rowid<?))`
		args = append(args, created, created, rowid)
	}
	args = append(args, limit+1)
	rows, err := s.db.Query(`SELECT `+messageColumns+` FROM messages WHERE conversation_id=?`+cursor+`
		ORDER BY created DESC, rowid DESC LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []domain.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore, nil
}
