package store

// replysenderid.go backfills domain.Reply.SenderID on a reply quote
// stored before that field existed: RefreshSenderName (see
// sendernames.go) needs it to scope a sender's name correction to the
// quote that actually names them, rather than trusting a message's
// remote id, which is only unique within one conversation. resolveReply
// fills it in for every reply from here on; this is the one-time
// migration step that catches up data already on disk.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// replyRow is one candidate row backfillReplySenderIDs considers: its
// rowid and conversation, for resolving the quote within that same
// conversation, and its reply_to column exactly as stored.
type replyRow struct {
	rowid          int64
	conversationID string
	replyTo        string
}

// scanReplyRow reads one row selected by backfillReplySenderIDs's own
// query.
func scanReplyRow(row scanner) (replyRow, error) {
	var r replyRow
	err := row.Scan(&r.rowid, &r.conversationID, &r.replyTo)

	return r, err
}

// repairReplySenderID fills in a reply's own quoted sender id by
// resolving its RemoteID against a message already stored in
// conversationID - the same lookup resolveReply does for a brand new
// reply, scoped the same way. changed is false, and replyTo is returned
// unchanged, when it carries no RemoteID, already has a SenderID, or
// the quoted message cannot be found in conversationID: left empty
// rather than guessed from a different conversation, exactly as an
// unresolved reply is left today.
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
// before that field existed, the one-time migration step that catches
// up data already on disk; resolveReply's own resolution keeps
// guaranteeing this for every reply from now on. Idempotent: a reply
// already carrying a SenderID, or one whose quote still cannot be
// resolved, is left exactly as it is on a repeated run.
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
