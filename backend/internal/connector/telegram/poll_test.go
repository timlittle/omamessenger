package telegram

import (
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollAnswer builds one of a poll's raw options, the way Telegram sends
// them: text plus the opaque bytes sendVote must echo back.
func pollAnswer(text string, option byte) *tg.PollAnswer {
	return &tg.PollAnswer{Text: tg.TextWithEntities{Text: text}, Option: []byte{option}}
}

func TestMedia_Poll(t *testing.T) {
	t.Parallel()

	poll := tg.Poll{
		Question:       tg.TextWithEntities{Text: "Lunch?"},
		MultipleChoice: true,
		Answers:        []tg.PollAnswerClass{pollAnswer("Pizza", 0), pollAnswer("Salad", 1)},
	}
	results := tg.PollResults{}
	results.SetTotalVoters(5)
	results.SetResults([]tg.PollAnswerVoters{
		{Option: []byte{0}, Voters: 3, Chosen: true},
		{Option: []byte{1}, Voters: 2},
	})

	got := media(&tg.MessageMediaPoll{Poll: poll, Results: results})
	if got == nil || got.Kind != domain.MediaPoll || got.Poll == nil {
		t.Fatalf("media = %+v, want a poll", got)
	}

	p := got.Poll
	if p.Question != "Lunch?" || !p.MultipleChoice || p.TotalVoters != 5 {
		t.Errorf("poll = %+v, want Lunch? multiple-choice with 5 voters", p)
	}
	if len(p.Options) != 2 || p.Options[0].Text != "Pizza" || p.Options[0].Votes != 3 || !p.Options[0].Chosen {
		t.Errorf("options[0] = %+v, want Pizza with 3 votes, chosen", p.Options[0])
	}
	if p.Options[1].Text != "Salad" || p.Options[1].Votes != 2 || p.Options[1].Chosen {
		t.Errorf("options[1] = %+v, want Salad with 2 votes, not chosen", p.Options[1])
	}
}

func TestPollUpdate_WithNoPollObjectKeepsOnlyTheTally(t *testing.T) {
	t.Parallel()

	// Telegram's live update can arrive with no poll object at all, when
	// it assumes the client already cached it; pollUpdate still reports
	// whatever tallies the update does carry.
	update := &tg.UpdateMessagePoll{}
	results := tg.PollResults{}
	results.SetTotalVoters(1)
	results.SetResults([]tg.PollAnswerVoters{{Option: []byte{0}, Voters: 1, Chosen: true}})
	update.Results = results

	got := pollUpdate(update)
	if got.Question != "" || got.TotalVoters != 1 {
		t.Errorf("pollUpdate = %+v, want no question, just the tally", got)
	}
	if len(got.Options) != 1 || got.Options[0].Text != "" || got.Options[0].Votes != 1 || !got.Options[0].Chosen {
		t.Errorf("options = %+v, want one option with no text and its tally", got.Options)
	}
}

func TestPollOptionID_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, option := range [][]byte{{0}, {1}, {0, 1, 2, 3}} {
		id := pollOptionID(option)
		got, err := pollOptionBytes(id)
		if err != nil || string(got) != string(option) {
			t.Errorf("pollOptionBytes(pollOptionID(%v)) = %v, %v", option, got, err)
		}
	}
}
