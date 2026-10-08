package telegram

// poll.go normalizes Telegram's poll media and its live updates into
// ours, and turns a chosen option back into the raw bytes
// messages.sendVote needs (see vote.go). Telegram identifies an option
// by an opaque byte string it hands out with the poll and expects back
// unchanged when voting, rather than by a stable id of its own, so
// pollOptionID just encodes those bytes for the protocol and the UI.

import (
	"encoding/base64"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollMedia normalizes a Telegram poll or quiz, with whatever results
// it already carries, into ours.
func pollMedia(m *tg.MessageMediaPoll) *domain.Media {
	poll := pollFrom(m.Poll, m.Results)

	return &domain.Media{Kind: domain.MediaPoll, Poll: &poll}
}

// pollUpdate normalizes a live poll update into ours. Telegram sends
// the poll's own question and options again only the first time a
// client sees it, leaving later updates to carry tallies alone; see
// domain.MergePoll for how the store keeps the first report's question
// and option text across those.
func pollUpdate(u *tg.UpdateMessagePoll) domain.Poll {
	poll, _ := u.GetPoll()

	return pollFrom(poll, u.Results)
}

// pollFrom builds a poll from Telegram's own poll and results objects,
// shared by pollMedia and pollUpdate so neither can disagree about how
// an option's tally is read. p is the zero Poll for a live update that
// assumes the client already cached the question and options (see
// pollUpdate): optionsFromResults then builds options straight from the
// tally alone, each with no text of its own, for domain.MergePoll to
// fill in from what was stored earlier.
func pollFrom(p tg.Poll, results tg.PollResults) domain.Poll {
	resultsList, _ := results.GetResults()
	tallies := make(map[string]tg.PollAnswerVoters, len(resultsList))
	for _, r := range resultsList {
		tallies[pollOptionID(r.Option)] = r
	}

	options := optionsFromResults(resultsList)
	if len(p.Answers) > 0 {
		options = optionsFromAnswers(p.Answers, tallies)
	}

	totalVoters, _ := results.GetTotalVoters()

	return domain.Poll{
		Question: p.Question.Text, Options: options,
		MultipleChoice: p.MultipleChoice, Quiz: p.Quiz, Closed: p.Closed,
		TotalVoters: totalVoters,
	}
}

// optionsFromAnswers builds a poll's options from its own answer list,
// the authoritative text and order, folding in each one's tally.
func optionsFromAnswers(answers []tg.PollAnswerClass, tallies map[string]tg.PollAnswerVoters) []domain.PollOption {
	options := make([]domain.PollOption, 0, len(answers))
	for _, a := range answers {
		ans, ok := a.(*tg.PollAnswer)
		if !ok {
			continue
		}

		id := pollOptionID(ans.Option)
		opt := domain.PollOption{ID: id, Text: ans.Text.Text}
		if r, ok := tallies[id]; ok {
			opt.Votes, opt.Chosen, opt.Correct = r.Voters, r.Chosen, r.Correct
		}
		options = append(options, opt)
	}

	return options
}

// optionsFromResults builds a poll's options straight from its tally
// alone, each with no text of its own, for an update that carries no
// poll object (see pollFrom).
func optionsFromResults(results []tg.PollAnswerVoters) []domain.PollOption {
	options := make([]domain.PollOption, len(results))
	for i, r := range results {
		options[i] = domain.PollOption{ID: pollOptionID(r.Option), Votes: r.Voters, Chosen: r.Chosen, Correct: r.Correct}
	}

	return options
}

// pollOptionID is the stable id this connector gives Telegram's raw
// option bytes.
func pollOptionID(option []byte) string {
	return base64.RawURLEncoding.EncodeToString(option)
}

// pollOptionBytes reverses pollOptionID, for Vote to rebuild the bytes
// messages.sendVote expects.
func pollOptionBytes(id string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(id)
}

// pollPlaceholder labels a poll with its question, or just names it a
// poll when Telegram reported none.
func pollPlaceholder(question string) string {
	if question == "" {
		return "[Poll]"
	}

	return "[Poll: " + question + "]"
}
