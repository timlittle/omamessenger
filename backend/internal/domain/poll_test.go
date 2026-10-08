package domain_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMergePoll_FirstUpdateIsReturnedAsGiven checks that a message with
// no poll yet just takes whatever the first update reports.
func TestMergePoll_FirstUpdateIsReturnedAsGiven(t *testing.T) {
	update := domain.Poll{Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}}}

	got := domain.MergePoll(nil, update)

	if got.Question != "Lunch?" || len(got.Options) != 1 || got.Options[0].Text != "Pizza" {
		t.Errorf("MergePoll(nil, update) = %+v, want update unchanged", got)
	}
}

// TestMergePoll_TalliesOnlyUpdateKeepsQuestionAndOptionText checks that
// a later update carrying only tallies, with no question or option
// text of its own, keeps what the poll was first reported with.
func TestMergePoll_TalliesOnlyUpdateKeepsQuestionAndOptionText(t *testing.T) {
	existing := domain.Poll{
		Question: "Lunch?", MultipleChoice: true,
		Options: []domain.PollOption{{ID: "a", Text: "Pizza"}, {ID: "b", Text: "Salad"}},
	}
	update := domain.Poll{
		Options:     []domain.PollOption{{ID: "a", Votes: 3, Chosen: true}, {ID: "b", Votes: 1}},
		TotalVoters: 4,
	}

	got := domain.MergePoll(&existing, update)

	if got.Question != "Lunch?" {
		t.Errorf("Question = %q, want kept from existing", got.Question)
	}
	if !got.MultipleChoice {
		t.Error("MultipleChoice = false, want kept from existing")
	}
	if got.Options[0].Text != "Pizza" || got.Options[1].Text != "Salad" {
		t.Errorf("Options = %+v, want text filled in from existing", got.Options)
	}
	if got.Options[0].Votes != 3 || !got.Options[0].Chosen {
		t.Errorf("Options[0] = %+v, want the update's own tally kept", got.Options[0])
	}
	if got.TotalVoters != 4 {
		t.Errorf("TotalVoters = %d, want 4", got.TotalVoters)
	}
}

// TestMergePoll_UpdateWithItsOwnQuestionReplacesNothing checks that an
// update which does carry a question, such as a poll's first live
// update, is used as given rather than merged with the stale copy.
func TestMergePoll_UpdateWithItsOwnQuestionReplacesNothing(t *testing.T) {
	existing := domain.Poll{Question: "Old?", MultipleChoice: true}
	update := domain.Poll{Question: "New?", MultipleChoice: false}

	got := domain.MergePoll(&existing, update)

	if got.Question != "New?" || got.MultipleChoice {
		t.Errorf("MergePoll kept the stale question or flag: %+v", got)
	}
}
