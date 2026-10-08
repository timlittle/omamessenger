package whatsapp

import (
	"reflect"
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
