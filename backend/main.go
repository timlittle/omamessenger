package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

// OmaMessenger's local API is transport agnostic. Service adapters implement
// Connector and publish normalized events into Store; QML never speaks a
// messaging protocol directly.
type Account struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}
type Conversation struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Service   string `json:"service"`
	Title     string `json:"title"`
	Preview   string `json:"preview"`
	Unread    int    `json:"unread"`
	Updated   string `json:"updated"`
}
type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Sender         string `json:"sender"`
	Text           string `json:"text"`
	Outgoing       bool   `json:"outgoing"`
	Created        string `json:"created"`
}
type Connector interface {
	Service() string
	Connect(context.Context, Account) error
	Send(context.Context, Conversation, string) (Message, error)
}
type Store struct{ db *sql.DB }

func openStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS accounts(id TEXT PRIMARY KEY,service TEXT NOT NULL,name TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'disconnected');
CREATE TABLE IF NOT EXISTS conversations(id TEXT PRIMARY KEY,account_id TEXT NOT NULL,service TEXT NOT NULL,title TEXT NOT NULL,preview TEXT NOT NULL DEFAULT '',unread INTEGER NOT NULL DEFAULT 0,updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS messages(id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,sender TEXT NOT NULL,text TEXT NOT NULL,outgoing INTEGER NOT NULL,created TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS messages_conversation ON messages(conversation_id,created);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) accounts() ([]Account, error) {
	rows, e := s.db.Query(`SELECT id,service,name,status FROM accounts ORDER BY name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		if e = rows.Scan(&a.ID, &a.Service, &a.Name, &a.Status); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) conversations(q string) ([]Conversation, error) {
	rows, e := s.db.Query(`SELECT c.id,c.account_id,c.service,c.title,c.preview,c.unread,c.updated FROM conversations c WHERE (?='' OR c.title LIKE ? OR c.preview LIKE ? OR EXISTS(SELECT 1 FROM messages m WHERE m.conversation_id=c.id AND m.text LIKE ?)) ORDER BY c.updated DESC`, q, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if e = rows.Scan(&c.ID, &c.AccountID, &c.Service, &c.Title, &c.Preview, &c.Unread, &c.Updated); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) messages(id string) ([]Message, error) {
	rows, e := s.db.Query(`SELECT id,conversation_id,sender,text,outgoing,created FROM messages WHERE conversation_id=? ORDER BY created DESC LIMIT 200`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if e = rows.Scan(&m.ID, &m.ConversationID, &m.Sender, &m.Text, &m.Outgoing, &m.Created); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
func (s *Store) addMessage(m Message) (Message, error) {
	tx, e := s.db.Begin()
	if e != nil {
		return m, e
	}
	defer tx.Rollback()
	if m.ID == "" {
		m.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	m.Created = time.Now().UTC().Format(time.RFC3339Nano)
	_, e = tx.Exec(`INSERT INTO messages(id,conversation_id,sender,text,outgoing,created) VALUES(?,?,?,?,?,?)`, m.ID, m.ConversationID, m.Sender, m.Text, m.Outgoing, m.Created)
	if e != nil {
		return m, e
	}
	_, e = tx.Exec(`UPDATE conversations SET preview=?,updated=?,unread=unread+? WHERE id=?`, m.Text, m.Created, boolInt(!m.Outgoing), m.ConversationID)
	if e != nil {
		return m, e
	}
	if e = tx.Commit(); e != nil {
		return m, e
	}
	return m, nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func loadAPIToken(configDir string) (string, error) {
	if token := os.Getenv("OMA_TOKEN"); token != "" {
		return token, nil
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "api.token")
	data, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0600); err != nil {
		return "", err
	}
	return token, nil
}

func main() {
	base, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	path := os.Getenv("OMA_DB")
	if path == "" {
		path = filepath.Join(base, "omamessenger", "messages.db")
	}
	token, err := loadAPIToken(filepath.Join(base, "omamessenger"))
	if err != nil {
		log.Fatal(err)
	}
	store, err := openStore(path)
	if err != nil {
		log.Fatal(err)
	}
	defer store.db.Close()
	mux := http.NewServeMux()
	jsonOut := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		v, e := store.accounts()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, v)
	})
	mux.HandleFunc("POST /api/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		var a Account
		if json.NewDecoder(r.Body).Decode(&a) != nil || a.ID == "" || a.Name == "" || (a.Service != "whatsapp" && a.Service != "telegram") {
			http.Error(w, "provide id, name and service (whatsapp or telegram)", 400)
			return
		}
		if a.Status == "" {
			a.Status = "needs-auth"
		}
		_, e := store.db.Exec(`INSERT INTO accounts(id,service,name,status) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET service=excluded.service,name=excluded.name`, a.ID, a.Service, a.Name, a.Status)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOut(w, a)
	})
	mux.HandleFunc("POST /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		var c Conversation
		if json.NewDecoder(r.Body).Decode(&c) != nil || c.ID == "" || c.AccountID == "" || c.Title == "" || (c.Service != "whatsapp" && c.Service != "telegram") {
			http.Error(w, "provide id, accountId, service and title", 400)
			return
		}
		var accountService string
		e := store.db.QueryRow(`SELECT service FROM accounts WHERE id=?`, c.AccountID).Scan(&accountService)
		if e != nil || accountService != c.Service {
			http.Error(w, "account not found or service mismatch", 400)
			return
		}
		c.Updated = time.Now().UTC().Format(time.RFC3339Nano)
		_, e = store.db.Exec(`INSERT INTO conversations(id,account_id,service,title,preview,unread,updated) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, c.ID, c.AccountID, c.Service, c.Title, "", 0, c.Updated)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOut(w, c)
	})
	mux.HandleFunc("GET /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		v, e := store.conversations(strings.TrimSpace(r.URL.Query().Get("q")))
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, v)
	})
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		v, e := store.messages(r.PathValue("id"))
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, v)
	})
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" {
			http.Error(w, "text is required", 400)
			return
		}
		var m Message
		m.ConversationID = r.PathValue("id")
		m.Sender = "You"
		m.Text = strings.TrimSpace(in.Text)
		m.Outgoing = true
		var e error
		m, e = store.addMessage(m)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, m)
	})
	mux.HandleFunc("POST /api/v1/events/message", func(w http.ResponseWriter, r *http.Request) {
		var m Message
		if json.NewDecoder(r.Body).Decode(&m) != nil || m.ConversationID == "" || strings.TrimSpace(m.Text) == "" {
			http.Error(w, "conversationId and text are required", http.StatusBadRequest)
			return
		}
		m.Outgoing = false
		if m.Sender == "" {
			m.Sender = "New message"
		}
		var err error
		m, err = store.addMessage(m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = exec.Command("notify-send", "--app-name=OmaMessenger", m.Sender, m.Text).Start()
		jsonOut(w, m)
	})
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		_, e := store.db.Exec(`UPDATE conversations SET unread=0 WHERE id=?`, r.PathValue("id"))
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	port := os.Getenv("OMA_PORT")
	if port == "" {
		port = "43821"
	}
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: withHeaders(mux, token), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("OmaMessenger API listening on %s (database %s)", srv.Addr, path)
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
func withHeaders(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		if r.URL.Path != "/api/v1/health" {
			expected := "Bearer " + token
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expected)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
