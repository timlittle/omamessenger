package domain

// Poll is a poll's question, options and tallies, carried by a
// message's Media when Media.Kind is MediaPoll. TotalVoters is the
// service's own count of people who voted, which can exceed the sum of
// Options' Votes when MultipleChoice lets one voter pick several.
type Poll struct {
	Question       string       `json:"question"`
	Options        []PollOption `json:"options"`
	MultipleChoice bool         `json:"multipleChoice,omitempty"`
	Quiz           bool         `json:"quiz,omitempty"`
	Closed         bool         `json:"closed,omitempty"`
	TotalVoters    int          `json:"totalVoters,omitempty"`
}

// PollOption is one choice in a poll, with its current tally. Chosen is
// true when the signed-in user picked it. Correct is set only for a
// quiz, and only once the signed-in user has voted or the poll closed:
// a service that revealed the right answer any earlier would spoil it,
// so a connector leaves every option's Correct false until then.
type PollOption struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Votes   int    `json:"votes"`
	Chosen  bool   `json:"chosen,omitempty"`
	Correct bool   `json:"correct,omitempty"`
}

// MergePoll folds a tallies-only update into the poll already stored
// for a message: an empty Question, or an empty Text on one of update's
// options, means "unchanged" rather than "cleared", since Telegram's
// own poll update only resends the question and option text the first
// time a client sees a poll, and WhatsApp's vote protocol never sends
// them at all. existing is nil the first time a message gets a poll,
// in which case update is returned as given.
func MergePoll(existing *Poll, update Poll) Poll {
	if existing == nil {
		return update
	}

	if update.Question == "" {
		update.Question, update.MultipleChoice, update.Quiz = existing.Question, existing.MultipleChoice, existing.Quiz
	}

	byID := make(map[string]PollOption, len(existing.Options))
	for _, o := range existing.Options {
		byID[o.ID] = o
	}

	for i, o := range update.Options {
		if o.Text == "" {
			if prev, ok := byID[o.ID]; ok {
				o.Text = prev.Text
			}
		}
		update.Options[i] = o
	}

	return update
}
