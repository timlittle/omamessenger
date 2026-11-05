package signal

import (
	"testing"

	"github.com/google/uuid"
)

// newTestPollConnector returns a connector with an in-memory media
// store wired in, enough for trackPoll, recordVote and closePoll to
// act on.
func newTestPollConnector(t *testing.T) *Connector {
	t.Helper()

	c := &Connector{media: newTestMediaStore(t)}

	return c
}

// TestPoll_VoteAndClose checks that votes tally by option index, this
// account's own vote is marked chosen, and terminate closes the poll.
func TestPoll_VoteAndClose(t *testing.T) {
	t.Parallel()

	c := newTestPollConnector(t)
	self := uuid.New()
	other := uuid.New()
	c.selfACI = self

	c.trackPoll(t.Context(), "conv1", "msg1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: false})

	poll, ok := c.recordVote(t.Context(), "conv1", "msg1", self, []int{0})
	if !ok {
		t.Fatal("recordVote reported the poll not found")
	}
	if poll.Options[0].Votes != 1 || !poll.Options[0].Chosen {
		t.Errorf("after own vote: %+v", poll.Options[0])
	}

	poll, ok = c.recordVote(t.Context(), "conv1", "msg1", other, []int{0})
	if !ok {
		t.Fatal("recordVote reported the poll not found")
	}
	if poll.Options[0].Votes != 2 || poll.TotalVoters != 2 {
		t.Errorf("after second vote: %+v", poll)
	}

	poll, ok = c.closePoll(t.Context(), "conv1", "msg1")
	if !ok || !poll.Closed {
		t.Errorf("closePoll: ok=%t poll=%+v", ok, poll)
	}
}

// TestPoll_UnknownMessage checks that a vote or terminate for a poll
// this run never saw created is reported not found, rather than
// fabricating an empty poll.
func TestPoll_UnknownMessage(t *testing.T) {
	t.Parallel()

	c := newTestPollConnector(t)

	if _, ok := c.recordVote(t.Context(), "conv1", "missing", uuid.New(), []int{0}); ok {
		t.Error("recordVote found an untracked poll")
	}
	if _, ok := c.closePoll(t.Context(), "conv1", "missing"); ok {
		t.Error("closePoll found an untracked poll")
	}
}

// TestPoll_RevoteReplaces checks that a second vote from the same
// voter replaces their first, since Signal always sends a voter's
// whole current choice rather than a delta.
func TestPoll_RevoteReplaces(t *testing.T) {
	t.Parallel()

	c := newTestPollConnector(t)
	voter := uuid.New()

	c.trackPoll(t.Context(), "conv1", "msg1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: true})
	c.recordVote(t.Context(), "conv1", "msg1", voter, []int{0})
	poll, _ := c.recordVote(t.Context(), "conv1", "msg1", voter, []int{1})

	if poll.Options[0].Votes != 0 || poll.Options[1].Votes != 1 {
		t.Errorf("after revote: %+v", poll.Options)
	}
}

// TestPoll_SurvivesARestart checks that a poll's options and votes,
// saved by one connector instance, still resolve to the right tally
// through a second one built around the same on-disk media store,
// standing in for a helper restart.
func TestPoll_SurvivesARestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	before, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	self := uuid.New()

	first := &Connector{media: before, selfACI: self}
	first.trackPoll(t.Context(), "conv1", "msg1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: false})
	first.recordVote(t.Context(), "conv1", "msg1", self, []int{0})
	if err := before.close(); err != nil {
		t.Fatal(err)
	}

	after, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = after.close() }()

	second := &Connector{media: after, selfACI: self}
	poll, ok := second.closePoll(t.Context(), "conv1", "msg1")
	if !ok || !poll.Closed || poll.Options[0].Votes != 1 || !poll.Options[0].Chosen {
		t.Errorf("poll after restart = %+v, ok=%t", poll, ok)
	}
}

// TestPoll_ClosedMediaStore checks that a vote or terminate against a
// closed media store is reported not found, rather than panicking or
// fabricating a poll.
func TestPoll_ClosedMediaStore(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	c := &Connector{media: media}
	c.trackPoll(t.Context(), "conv1", "msg1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: false})
	if err := media.close(); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.recordVote(t.Context(), "conv1", "msg1", uuid.New(), []int{0}); ok {
		t.Error("recordVote succeeded against a closed media store")
	}
	if _, ok := c.closePoll(t.Context(), "conv1", "msg1"); ok {
		t.Error("closePoll succeeded against a closed media store")
	}
}

// TestPoll_NoMediaStore checks that tracking, voting and closing a
// poll with no media store wired in are harmless no-ops, rather than
// panicking, and are reported not found.
func TestPoll_NoMediaStore(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	c.trackPoll(t.Context(), "conv1", "msg1", newPoll{question: "Pizza?", options: []string{"Yes", "No"}, multipleChoice: false})

	if _, ok := c.recordVote(t.Context(), "conv1", "msg1", uuid.New(), []int{0}); ok {
		t.Error("recordVote found a poll with no media store")
	}
	if _, ok := c.closePoll(t.Context(), "conv1", "msg1"); ok {
		t.Error("closePoll found a poll with no media store")
	}
}
