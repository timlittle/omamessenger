package whatsapp

// organizehealthstore.go persists, in the same per-account database as
// the media store and message keys (see storage.go), which app-state
// patch types Connector.setOrganizeLocalOnly has found permanently
// unverifiable (see organizehealth.go): without this, a helper restart
// would have to rediscover the same corrupt-and-rejected failure from
// scratch, repeating its own expensive full resync, before pin,
// archive, mark-read and delete-for-me could go back to skipping
// WhatsApp's server for it.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/appstate"
)

// organizeHealthSchema creates the table organizeUnsyncable and
// putOrganizeUnsyncable use, if it does not already exist.
const organizeHealthSchema = `CREATE TABLE IF NOT EXISTS organize_health (
	patch_type TEXT PRIMARY KEY,
	unsyncable INTEGER NOT NULL
)`

// ensureOrganizeHealthTable creates the organize_health table in db, if
// it does not already exist.
func ensureOrganizeHealthTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, organizeHealthSchema); err != nil {
		return fmt.Errorf("whatsapp: prepare organize health store: %w", err)
	}

	return nil
}

// organizeUnsyncable returns whether patchType was last recorded
// unsyncable, false when nothing has ever been recorded for it.
func (m *mediaStore) organizeUnsyncable(ctx context.Context, patchType appstate.WAPatchName) (bool, error) {
	var unsyncable bool

	q := `SELECT unsyncable FROM organize_health WHERE patch_type = ?`
	err := m.db.QueryRowContext(ctx, q, string(patchType)).Scan(&unsyncable)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("whatsapp: load organize health: %w", err)
	}

	return unsyncable, nil
}

// putOrganizeUnsyncable records whether patchType can currently be
// trusted to reach WhatsApp's server, replacing whatever was saved for
// it before.
func (m *mediaStore) putOrganizeUnsyncable(ctx context.Context, patchType appstate.WAPatchName, unsyncable bool) error {
	q := `INSERT INTO organize_health (patch_type, unsyncable) VALUES (?, ?)
		ON CONFLICT (patch_type) DO UPDATE SET unsyncable = excluded.unsyncable`

	if _, err := m.db.ExecContext(ctx, q, string(patchType), unsyncable); err != nil {
		return fmt.Errorf("whatsapp: save organize health: %w", err)
	}

	return nil
}
