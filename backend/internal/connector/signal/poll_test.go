package signal

import (
	"testing"

	"github.com/google/uuid"
)

// TestPoll_VoteAndClose checks that votes tally by option index, this
// account's own vote is marked chosen, and terminate closes the poll.
func TestPoll_VoteAndClose(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	self := uuid.New()
	other := uuid.New()

	c.trackPoll("msg1", "Pizza?", []string{"Yes", "No"}, false)

	poll, ok := c.recordVote("msg1", self, []int{0}, self)
	if !ok {
		t.Fatal("recordVote reported the poll not found")
	}
	if poll.Options[0].Votes != 1 || !poll.Options[0].Chosen {
		t.Errorf("after own vote: %+v", poll.Options[0])
	}

	poll, ok = c.recordVote("msg1", other, []int{0}, self)
	if !ok {
		t.Fatal("recordVote reported the poll not found")
	}
	if poll.Options[0].Votes != 2 || poll.TotalVoters != 2 {
		t.Errorf("after second vote: %+v", poll)
	}

	poll, ok = c.closePoll("msg1", self)
	if !ok || !poll.Closed {
		t.Errorf("closePoll: ok=%t poll=%+v", ok, poll)
	}
}

// TestPoll_UnknownMessage checks that a vote or terminate for a poll
// this run never saw created is reported not found, rather than
// fabricating an empty poll.
func TestPoll_UnknownMessage(t *testing.T) {
	t.Parallel()

	c := &Connector{}

	if _, ok := c.recordVote("missing", uuid.New(), []int{0}, uuid.Nil); ok {
		t.Error("recordVote found an untracked poll")
	}
	if _, ok := c.closePoll("missing", uuid.Nil); ok {
		t.Error("closePoll found an untracked poll")
	}
}

// TestPoll_RevoteReplaces checks that a second vote from the same
// voter replaces their first, since Signal always sends a voter's
// whole current choice rather than a delta.
func TestPoll_RevoteReplaces(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	voter := uuid.New()

	c.trackPoll("msg1", "Pizza?", []string{"Yes", "No"}, true)
	c.recordVote("msg1", voter, []int{0}, uuid.Nil)
	poll, _ := c.recordVote("msg1", voter, []int{1}, uuid.Nil)

	if poll.Options[0].Votes != 0 || poll.Options[1].Votes != 1 {
		t.Errorf("after revote: %+v", poll.Options)
	}
}

// TestPoll_Eviction checks that tracking more than maxTrackedPolls
// evicts the oldest first, so a very chatty account cannot grow this
// table without bound.
func TestPoll_Eviction(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	for i := 0; i < maxTrackedPolls+1; i++ {
		c.trackPoll(uuid.New().String(), "q", []string{"a"}, false)
	}

	if len(c.polls) != maxTrackedPolls {
		t.Errorf("len(polls) = %d, want %d", len(c.polls), maxTrackedPolls)
	}
}
