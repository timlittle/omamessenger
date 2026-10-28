package store

// conversations_pin.go holds a conversation's pin and archive
// bookkeeping: pin order, moving a pin, a dialog sync's own organize
// report, and remembering a pin across an archive so unarchiving can
// give it back (see docs/decisions.md).

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

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
