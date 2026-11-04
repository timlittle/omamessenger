package store

// replysenderid.go backfills domain.Reply.SenderID on reply quotes
// stored before that field existed, so RefreshSenderName can scope a
// name correction to the quote that actually names the sender.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// replyRow is one candidate row backfillReplySenderIDs considers.
type replyRow struct {
	rowid          int64
	conversationID string
	replyTo        string
}

// scanReplyRow reads one row selected by backfillReplySenderIDs.
func scanReplyRow(row scanner) (replyRow, error) {
	var r replyRow
	err := row.Scan(&r.rowid, &r.conversationID, &r.replyTo)

	return r, err
}

// repairReplySenderID fills in a reply's quoted sender id by resolving
// its RemoteID within conversationID. changed is false, and replyTo
// unchanged, when it already has a SenderID or the quote can't be
// resolved there.
func repairReplySenderID(ctx context.Context, tx *sql.Tx, conversationID, replyTo string) (repaired string, changed bool, err error) {
	var reply domain.Reply
	if unmarshalErr := json.Unmarshal([]byte(replyTo), &reply); unmarshalErr != nil {
		return replyTo, false, nil //nolint:nilerr // deliberate: a row this malformed can never have come from the store's own encodeReply; leave it as stored rather than fail the whole backfill over one unparsable row
	}
	if reply.RemoteID == "" || reply.SenderID != "" {
		return replyTo, false, nil
	}

	var senderID string
	err = tx.QueryRowContext(ctx, `
		SELECT sender_id FROM messages WHERE conversation_id=? AND remote_id=? AND remote_id<>''`,
		conversationID, reply.RemoteID).Scan(&senderID)
	if errors.Is(err, sql.ErrNoRows) || senderID == "" {
		return replyTo, false, nil
	}
	if err != nil {
		return replyTo, false, err
	}

	reply.SenderID = senderID
	out, err := json.Marshal(reply)
	if err != nil {
		return replyTo, false, err
	}

	return string(out), true, nil
}

// backfillReplySenderIDs fills in SenderID on every reply quote stored
// before that field existed. Idempotent: a reply already resolved, or
// one that still can't be, is left unchanged on a repeated run.
func backfillReplySenderIDs(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT rowid, conversation_id, reply_to FROM messages WHERE reply_to<>''`)
	if err != nil {
		return err
	}

	candidates, err := scanAll(rows, scanReplyRow)
	if err != nil {
		return err
	}

	for _, c := range candidates {
		repaired, changed, err := repairReplySenderID(ctx, tx, c.conversationID, c.replyTo)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}

		if _, err := tx.ExecContext(ctx, `UPDATE messages SET reply_to=? WHERE rowid=?`, repaired, c.rowid); err != nil {
			return err
		}
	}

	return nil
}
