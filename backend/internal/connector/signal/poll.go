package signal

import (
	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// maxTrackedPolls bounds how many open polls one run remembers the
// options and votes of, the same way the WhatsApp connector bounds its
// own in-memory reaction and undecryptable tables, so a very chatty
// account cannot grow this without bound.
const maxTrackedPolls = 256

// pollState is a poll's options and the votes cast for it so far,
// since Signal's own vote and terminate events carry no option text of
// their own: this connector is the only place that ever sees all three
// (create, vote, terminate) together, the same way it is the only
// place that ever sees a reaction's running tally.
type pollState struct {
	question       string
	options        []string
	multipleChoice bool
	closed         bool
	votes          map[uuid.UUID][]int // voter aci to their chosen option indexes
}

// trackPoll remembers a newly created poll's options under
// messageRemoteID, evicting the oldest tracked poll first if this run
// is already at maxTrackedPolls.
func (c *Connector) trackPoll(messageRemoteID, question string, options []string, multipleChoice bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if c.polls == nil {
		c.polls = map[string]*pollState{}
	}
	if _, exists := c.polls[messageRemoteID]; !exists && len(c.polls) >= maxTrackedPolls {
		oldest, rest := c.pollOrder[0], c.pollOrder[1:]
		delete(c.polls, oldest)
		c.pollOrder = rest
	}

	c.polls[messageRemoteID] = &pollState{question: question, options: options, multipleChoice: multipleChoice, votes: map[uuid.UUID][]int{}}
	c.pollOrder = append(c.pollOrder, messageRemoteID)
}

// recordVote folds voter's chosen option indexes into the poll tracked
// under messageRemoteID, replacing any previous vote of theirs, since
// Signal's vote protocol always sends a voter's complete current
// choice rather than a delta. It reports the poll's current tallies,
// or found false when this run never saw that poll created.
func (c *Connector) recordVote(messageRemoteID string, voter uuid.UUID, optionIndexes []int, self uuid.UUID) (domain.Poll, bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	state, ok := c.polls[messageRemoteID]
	if !ok {
		return domain.Poll{}, false
	}

	state.votes[voter] = optionIndexes

	return pollSnapshot(state, self), true
}

// closePoll marks the poll tracked under messageRemoteID closed and
// reports its final tallies, or found false when this run never saw
// that poll created.
func (c *Connector) closePoll(messageRemoteID string, self uuid.UUID) (domain.Poll, bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	state, ok := c.polls[messageRemoteID]
	if !ok {
		return domain.Poll{}, false
	}

	state.closed = true

	return pollSnapshot(state, self), true
}

// pollSnapshot builds the poll's current public shape from its
// tracked state: each option's vote count and, for self's own vote,
// which are Chosen.
func pollSnapshot(state *pollState, self uuid.UUID) domain.Poll {
	options := make([]domain.PollOption, len(state.options))
	for i, text := range state.options {
		options[i] = domain.PollOption{ID: formatOptionID(i), Text: text}
	}

	voters := map[uuid.UUID]bool{}
	for voter, chosen := range state.votes {
		voters[voter] = true
		for _, idx := range chosen {
			if idx < 0 || idx >= len(options) {
				continue
			}

			options[idx].Votes++
			if voter == self {
				options[idx].Chosen = true
			}
		}
	}

	return domain.Poll{
		Question:       state.question,
		Options:        options,
		MultipleChoice: state.multipleChoice,
		Closed:         state.closed,
		TotalVoters:    len(voters),
	}
}
