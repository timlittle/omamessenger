package whatsapp

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestPollStore_PutAndTally(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{
		Question: "Lunch?", MultipleChoice: true,
		Options: []domain.PollOption{{ID: "a", Text: "Pizza"}, {ID: "b", Text: "Salad"}},
	}

	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	shell, found, err := store.pollShell(ctx, "chat", "m1")
	if err != nil || !found {
		t.Fatalf("pollShell = %+v, found %t, %v", shell, found, err)
	}
	if shell.Question != "Lunch?" || !shell.MultipleChoice || !reflect.DeepEqual(shell.Options, poll.Options) {
		t.Errorf("shell = %+v, want %+v", shell, poll)
	}

	if err := store.setVotes(ctx, "chat", "m1", "alex", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(ctx, "chat", "m1", "self", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}

	tally, found, err := store.pollTally(ctx, "chat", "m1", "self")
	if err != nil || !found {
		t.Fatalf("pollTally = %+v, found %t, %v", tally, found, err)
	}
	if tally.TotalVoters != 2 {
		t.Errorf("TotalVoters = %d, want 2", tally.TotalVoters)
	}
	if tally.Options[0].Votes != 2 || !tally.Options[0].Chosen {
		t.Errorf("Pizza = %+v, want 2 votes, chosen by self", tally.Options[0])
	}
	if tally.Options[1].Votes != 1 || !tally.Options[1].Chosen {
		t.Errorf("Salad = %+v, want 1 vote, chosen by self", tally.Options[1])
	}

	tallyForAlex, _, err := store.pollTally(ctx, "chat", "m1", "alex")
	if err != nil {
		t.Fatal(err)
	}
	if tallyForAlex.Options[1].Chosen {
		t.Errorf("Salad chosen for alex, who never voted for it: %+v", tallyForAlex.Options[1])
	}
}

func TestPollStore_SetVotesReplacesAVotersEarlierChoice(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}, {ID: "b", Text: "Salad"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	if err := store.setVotes(ctx, "chat", "m1", "self", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(ctx, "chat", "m1", "self", []string{"b"}); err != nil {
		t.Fatal(err)
	}

	tally, _, err := store.pollTally(ctx, "chat", "m1", "self")
	if err != nil {
		t.Fatal(err)
	}
	if tally.Options[0].Votes != 0 || tally.Options[0].Chosen {
		t.Errorf("Pizza = %+v, want the earlier vote replaced", tally.Options[0])
	}
	if tally.Options[1].Votes != 1 || !tally.Options[1].Chosen {
		t.Errorf("Salad = %+v, want the new vote", tally.Options[1])
	}
}

func TestPollStore_SetVotesWithNoOptionsRetractsTheVote(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	if err := store.setVotes(ctx, "chat", "m1", "self", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.setVotes(ctx, "chat", "m1", "self", nil); err != nil {
		t.Fatal(err)
	}

	tally, _, err := store.pollTally(ctx, "chat", "m1", "self")
	if err != nil {
		t.Fatal(err)
	}
	if tally.TotalVoters != 0 || tally.Options[0].Votes != 0 {
		t.Errorf("tally = %+v, want the retracted vote gone", tally)
	}
}

// TestPutPoll_BoundsOptionCountBeforeStoring confirms a poll creation
// message's own option list, which carries no count limit of its own
// before this runs, never writes more than domain.MaxPollOptions rows
// to poll_options: without this, a hostile poll's own option list
// could grow this account's own database by as many rows as its
// sender's client cared to send, long before domain.CapMedia ever gets
// a chance to bound it on the way back out.
func TestPutPoll_BoundsOptionCountBeforeStoring(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()

	options := make([]domain.PollOption, domain.MaxPollOptions+10)
	for i := range options {
		options[i] = domain.PollOption{ID: fmt.Sprintf("o%d", i), Text: "option"}
	}

	if err := store.putPoll(ctx, "chat", "m1", domain.Poll{Question: "Q", Options: options}); err != nil {
		t.Fatal(err)
	}

	shell, found, err := store.pollShell(ctx, "chat", "m1")
	if err != nil || !found {
		t.Fatalf("pollShell = %+v, found %t, %v", shell, found, err)
	}
	if len(shell.Options) != domain.MaxPollOptions {
		t.Errorf("stored %d options, want at most %d", len(shell.Options), domain.MaxPollOptions)
	}
}

// TestPutPoll_DropsAnOversizedOptionID confirms an option whose own id
// runs past domain.MaxRemoteIDLength is never written at all: a real
// option's id is always exactly 64 hex characters (see poll.go's
// pollOptionID), so nothing legitimate is ever this long.
func TestPutPoll_DropsAnOversizedOptionID(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()

	huge := strings.Repeat("a", domain.MaxRemoteIDLength+1)
	poll := domain.Poll{Question: "Q", Options: []domain.PollOption{{ID: "ok", Text: "Fine"}, {ID: huge, Text: "Huge"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	shell, found, err := store.pollShell(ctx, "chat", "m1")
	if err != nil || !found {
		t.Fatalf("pollShell = %+v, found %t, %v", shell, found, err)
	}
	if len(shell.Options) != 1 || shell.Options[0].ID != "ok" {
		t.Errorf("stored options = %+v, want only the one with a plausible id", shell.Options)
	}
}

// TestSetVotes_BoundsOptionCountBeforeStoring confirms a live vote's
// own selected-option list, which carries no count limit of its own
// before this runs (see vote.go's handlePollVote), never writes more
// than domain.MaxPollOptions rows to poll_votes for a single vote.
func TestSetVotes_BoundsOptionCountBeforeStoring(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{Question: "Q", Options: []domain.PollOption{{ID: "a", Text: "A"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	ids := make([]string, domain.MaxPollOptions+10)
	for i := range ids {
		ids[i] = fmt.Sprintf("o%d", i)
	}

	if err := store.setVotes(ctx, "chat", "m1", "nadia", ids); err != nil {
		t.Fatal(err)
	}

	votersByOption, _, err := store.pollVoters(ctx, "chat", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(votersByOption) != domain.MaxPollOptions {
		t.Errorf("stored votes for %d distinct options, want at most %d", len(votersByOption), domain.MaxPollOptions)
	}
}

// TestSetVotes_DropsAnOversizedOptionID confirms a selected option id
// past domain.MaxRemoteIDLength is never written: a real vote's own
// option hash is always exactly 64 hex characters, so nothing
// legitimate is ever this long, and storing one anyway would only let
// a single hostile vote grow this account's own database without
// limit.
func TestSetVotes_DropsAnOversizedOptionID(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{Question: "Q", Options: []domain.PollOption{{ID: "a", Text: "A"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	huge := strings.Repeat("x", domain.MaxRemoteIDLength+1)
	if err := store.setVotes(ctx, "chat", "m1", "nadia", []string{"a", huge}); err != nil {
		t.Fatal(err)
	}

	votersByOption, _, err := store.pollVoters(ctx, "chat", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(votersByOption) != 1 {
		t.Errorf("stored votes for %d options, want only the one plausible id", len(votersByOption))
	}
}

// TestSetVotes_DropsAnOversizedVoterID confirms a vote reported for a
// voter id past domain.MaxRemoteIDLength is still recorded for its
// chosen option, but never under that oversized id itself: a sender's
// own JID has no length limit of its own before this runs (see
// live.go's reactorKey), and voter_id is never truncated for the same
// reason domain.CapRemoteID never truncates a reply quote's own
// RemoteID - a cut id could read as a different, real, shorter voter
// it was never actually cast by.
func TestSetVotes_DropsAnOversizedVoterID(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)
	ctx := t.Context()
	poll := domain.Poll{Question: "Q", Options: []domain.PollOption{{ID: "a", Text: "A"}}}
	if err := store.putPoll(ctx, "chat", "m1", poll); err != nil {
		t.Fatal(err)
	}

	huge := strings.Repeat("v", domain.MaxRemoteIDLength+1)
	if err := store.setVotes(ctx, "chat", "m1", huge, []string{"a"}); err != nil {
		t.Fatal(err)
	}

	votersByOption, _, err := store.pollVoters(ctx, "chat", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if votersByOption["a"][huge] {
		t.Error("stored the oversized voter id verbatim, want it dropped")
	}
}

func TestPollStore_PollShellReportsNotFound(t *testing.T) {
	t.Parallel()

	store := newTestMediaStore(t)

	_, found, err := store.pollShell(t.Context(), "chat", "missing")
	if err != nil || found {
		t.Fatalf("pollShell(missing) = found %t, %v; want not found", found, err)
	}

	_, found, err = store.pollTally(t.Context(), "chat", "missing", "self")
	if err != nil || found {
		t.Fatalf("pollTally(missing) = found %t, %v; want not found", found, err)
	}
}
