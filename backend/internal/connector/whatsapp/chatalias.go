package whatsapp

// chatalias.go keeps a conversation's remote id stable for its whole
// life, in the same per-account database as the media store and
// message keys (see storage.go and keys.go). WhatsApp addresses the
// same 1:1 chat by phone JID (PN) or by hidden id (LID) depending on
// which of its own internal paths delivered a given report, and
// whatsmeow only learns the LID-to-PN mapping for a given contact at
// some point after it starts seeing their messages, not before (see
// normalize.go's chatID). Without something of this connector's own
// to remember which form a chat was first reported under, a chat seen
// by LID before that mapping is known, then again by PN once it
// arrives, would otherwise look like two different chats.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// chatAliasSchema creates the table resolveChatAlias and putChatAlias
// use, if it does not already exist.
const chatAliasSchema = `CREATE TABLE IF NOT EXISTS chat_aliases (
	alias     TEXT PRIMARY KEY,
	canonical TEXT NOT NULL
)`

// ensureChatAliasTable creates the chat_aliases table in db, if it
// does not already exist.
func ensureChatAliasTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, chatAliasSchema); err != nil {
		return fmt.Errorf("whatsapp: prepare chat alias store: %w", err)
	}

	return nil
}

// resolveChatAlias returns the canonical remote id already recorded
// for any of aliases, the first match found, or "" when none of them
// has been seen before: a chat chatID is resolving for the first time,
// ever, under either of its known address forms.
func (m *mediaStore) resolveChatAlias(ctx context.Context, aliases []string) (string, error) {
	const q = `SELECT canonical FROM chat_aliases WHERE alias = ?`

	for _, alias := range aliases {
		var canonical string
		err := m.db.QueryRowContext(ctx, q, alias).Scan(&canonical)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("whatsapp: resolve chat alias: %w", err)
		}

		return canonical, nil
	}

	return "", nil
}

// putChatAlias records that alias resolves to canonical, replacing
// whatever was saved for it before.
func (m *mediaStore) putChatAlias(ctx context.Context, alias, canonical string) error {
	const q = `INSERT INTO chat_aliases (alias, canonical) VALUES (?, ?)
		ON CONFLICT (alias) DO UPDATE SET canonical = excluded.canonical`

	if _, err := m.db.ExecContext(ctx, q, alias, canonical); err != nil {
		return fmt.Errorf("whatsapp: save chat alias: %w", err)
	}

	return nil
}
