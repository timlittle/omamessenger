package whatsapp

// pollstore.go persists a poll's question and option text, and the
// options each voter has currently chosen, in the same per-account
// database as the media store (see storage.go). WhatsApp's vote
// protocol decrypts to option hashes alone, with no text and no
// running tally of its own (see vote.go's DecryptPollVote), so this is
// what lets a later vote still be shown against the right option, and
// a poll's tally still be recomputed after a restart with nothing of
// this run's own left in memory. whatsmeow's own per-device store
// separately keeps the secret a vote is decrypted with; this file never
// touches that.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollSchema creates the tables ensurePollTables installs, if they do
// not already exist.
const pollSchema = `
CREATE TABLE IF NOT EXISTS poll_meta (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	question        TEXT NOT NULL,
	multiple_choice INTEGER NOT NULL,
	PRIMARY KEY (conversation_id, message_id)
);
CREATE TABLE IF NOT EXISTS poll_options (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	option_id       TEXT NOT NULL,
	text            TEXT NOT NULL,
	position        INTEGER NOT NULL,
	PRIMARY KEY (conversation_id, message_id, option_id)
);
CREATE TABLE IF NOT EXISTS poll_votes (
	conversation_id TEXT NOT NULL,
	message_id      TEXT NOT NULL,
	voter_id        TEXT NOT NULL,
	option_id       TEXT NOT NULL,
	PRIMARY KEY (conversation_id, message_id, voter_id, option_id)
);`

// ensurePollTables creates poll_meta, poll_options and poll_votes in
// db, if they do not already exist.
func ensurePollTables(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, pollSchema); err != nil {
		return fmt.Errorf("whatsapp: prepare poll store: %w", err)
	}

	return nil
}

// putPoll saves a poll's question, multiple-choice flag and option
// text, replacing whatever was saved for it before. It is called once,
// when a poll creation message is first seen.
func (m *mediaStore) putPoll(ctx context.Context, conversationRemoteID, messageRemoteID string, poll domain.Poll) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("whatsapp: save poll: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // only reached after Commit fails or on an early return, nothing left to flush

	const meta = `INSERT INTO poll_meta (conversation_id, message_id, question, multiple_choice) VALUES (?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id) DO UPDATE SET question = excluded.question, multiple_choice = excluded.multiple_choice`
	if _, err := tx.ExecContext(ctx, meta, conversationRemoteID, messageRemoteID, poll.Question, poll.MultipleChoice); err != nil {
		return fmt.Errorf("whatsapp: save poll: %w", err)
	}

	const option = `INSERT INTO poll_options (conversation_id, message_id, option_id, text, position) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, message_id, option_id) DO UPDATE SET text = excluded.text, position = excluded.position`
	for i, o := range poll.Options {
		if _, err := tx.ExecContext(ctx, option, conversationRemoteID, messageRemoteID, o.ID, o.Text, i); err != nil {
			return fmt.Errorf("whatsapp: save poll: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("whatsapp: save poll: %w", err)
	}

	return nil
}

// pollShell returns a poll's question, multiple-choice flag and
// options, each with no tally yet, or false when no poll was saved for
// this message.
func (m *mediaStore) pollShell(ctx context.Context, conversationRemoteID, messageRemoteID string) (domain.Poll, bool, error) {
	const metaQuery = `SELECT question, multiple_choice FROM poll_meta WHERE conversation_id = ? AND message_id = ?`

	var poll domain.Poll
	row := m.db.QueryRowContext(ctx, metaQuery, conversationRemoteID, messageRemoteID)
	if err := row.Scan(&poll.Question, &poll.MultipleChoice); err != nil {
		if err == sql.ErrNoRows {
			return domain.Poll{}, false, nil
		}

		return domain.Poll{}, false, fmt.Errorf("whatsapp: load poll: %w", err)
	}

	options, err := m.pollOptions(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return domain.Poll{}, false, err
	}
	poll.Options = options

	return poll, true, nil
}

// pollOptions returns a poll's options, in the order they were saved
// with, each with no tally yet.
func (m *mediaStore) pollOptions(ctx context.Context, conversationRemoteID, messageRemoteID string) ([]domain.PollOption, error) {
	const q = `SELECT option_id, text FROM poll_options WHERE conversation_id = ? AND message_id = ? ORDER BY position`

	rows, err := m.db.QueryContext(ctx, q, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: load poll options: %w", err)
	}
	defer rows.Close()

	var options []domain.PollOption
	for rows.Next() {
		var o domain.PollOption
		if err := rows.Scan(&o.ID, &o.Text); err != nil {
			return nil, fmt.Errorf("whatsapp: load poll options: %w", err)
		}
		options = append(options, o)
	}

	return options, rows.Err()
}

// setVotes replaces voterID's selected options for a poll with
// optionIDs, an empty list retracting every earlier vote, the way a
// later PollUpdateMessage always carries the voter's complete,
// current selection rather than a diff.
func (m *mediaStore) setVotes(ctx context.Context, conversationRemoteID, messageRemoteID, voterID string, optionIDs []string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("whatsapp: save vote: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // only reached after Commit fails or on an early return, nothing left to flush

	const clear = `DELETE FROM poll_votes WHERE conversation_id = ? AND message_id = ? AND voter_id = ?`
	if _, err := tx.ExecContext(ctx, clear, conversationRemoteID, messageRemoteID, voterID); err != nil {
		return fmt.Errorf("whatsapp: save vote: %w", err)
	}

	const insert = `INSERT INTO poll_votes (conversation_id, message_id, voter_id, option_id) VALUES (?, ?, ?, ?)`
	for _, id := range optionIDs {
		if _, err := tx.ExecContext(ctx, insert, conversationRemoteID, messageRemoteID, voterID, id); err != nil {
			return fmt.Errorf("whatsapp: save vote: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("whatsapp: save vote: %w", err)
	}

	return nil
}

// pollTally rebuilds a poll's full state: its question, options and,
// for each option, how many distinct voters chose it and whether
// selfVoterID is one of them. found is false when no poll was saved
// for this message at all.
func (m *mediaStore) pollTally(ctx context.Context, conversationRemoteID, messageRemoteID, selfVoterID string) (domain.Poll, bool, error) {
	poll, found, err := m.pollShell(ctx, conversationRemoteID, messageRemoteID)
	if err != nil || !found {
		return domain.Poll{}, found, err
	}

	votersByOption, totalVoters, err := m.pollVoters(ctx, conversationRemoteID, messageRemoteID)
	if err != nil {
		return domain.Poll{}, false, err
	}

	for i, o := range poll.Options {
		voters := votersByOption[o.ID]
		poll.Options[i].Votes = len(voters)
		poll.Options[i].Chosen = voters[selfVoterID]
	}
	poll.TotalVoters = totalVoters

	return poll, true, nil
}

// pollVoters lists, for each option id, the set of voters who chose it,
// and how many distinct voters voted at all.
func (m *mediaStore) pollVoters(ctx context.Context, conversationRemoteID, messageRemoteID string) (map[string]map[string]bool, int, error) {
	const q = `SELECT option_id, voter_id FROM poll_votes WHERE conversation_id = ? AND message_id = ?`

	rows, err := m.db.QueryContext(ctx, q, conversationRemoteID, messageRemoteID)
	if err != nil {
		return nil, 0, fmt.Errorf("whatsapp: load votes: %w", err)
	}
	defer rows.Close()

	votersByOption := map[string]map[string]bool{}
	everyVoter := map[string]bool{}
	for rows.Next() {
		var optionID, voterID string
		if err := rows.Scan(&optionID, &voterID); err != nil {
			return nil, 0, fmt.Errorf("whatsapp: load votes: %w", err)
		}

		if votersByOption[optionID] == nil {
			votersByOption[optionID] = map[string]bool{}
		}
		votersByOption[optionID][voterID] = true
		everyVoter[voterID] = true
	}

	return votersByOption, len(everyVoter), rows.Err()
}
