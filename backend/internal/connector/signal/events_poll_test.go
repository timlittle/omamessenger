package signal

// events_poll_test.go covers handlePollVote and handlePollTerminate,
// split out of events_test.go to keep that file under the line limit;
// see its own comment.

import (
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"google.golang.org/protobuf/proto"
)

// TestHandlePollVote_InvalidTargetAuthor checks that a vote with a
// malformed target author is dropped.
func TestHandlePollVote_InvalidTargetAuthor(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: uuid.New(), ChatID: "x"}
	vote := &signalpb.DataMessage_PollVote{TargetAuthorAciBinary: []byte{1, 2, 3}}

	c.handlePollVote(t.Context(), sink, info, vote)

	if len(sink.Lines()) != 0 {
		t.Errorf("vote reported despite a malformed target author: %v", sink.Lines())
	}
}

// TestHandlePollVote_UnknownPoll checks that a vote for a poll this
// run never saw created is dropped rather than fabricating one.
func TestHandlePollVote_UnknownPoll(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	authorBytes, err := author.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: uuid.New(), ChatID: author.String()}
	vote := &signalpb.DataMessage_PollVote{TargetAuthorAciBinary: authorBytes, TargetSentTimestamp: proto.Uint64(99), OptionIndexes: []uint32{0}}

	c.handlePollVote(t.Context(), sink, info, vote)

	if len(sink.Lines()) != 0 {
		t.Errorf("vote reported for an untracked poll: %v", sink.Lines())
	}
}

// TestHandlePollTerminate_UnknownPoll checks the same for a terminate.
func TestHandlePollTerminate_UnknownPoll(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: author, ChatID: author.String()}
	terminate := &signalpb.DataMessage_PollTerminate{TargetSentTimestamp: proto.Uint64(99)}

	c.handlePollTerminate(t.Context(), sink, info, terminate)

	if len(sink.Lines()) != 0 {
		t.Errorf("terminate reported for an untracked poll: %v", sink.Lines())
	}
}

// TestHandleEvent_PollCreateVoteTerminate checks a poll's whole
// lifecycle through live events: creation, a vote and a terminate,
// each reported through PollUpdater once the vote and terminate find
// the poll this run tracked at creation.
func TestHandleEvent_PollCreateVoteTerminate(t *testing.T) {
	t.Parallel()

	creator := uuid.New()
	voter := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	createInfo := events.MessageInfo{Sender: creator, ChatID: creator.String()}
	create := &signalpb.DataMessage{
		Timestamp:  proto.Uint64(5),
		PollCreate: &signalpb.DataMessage_PollCreate{Question: proto.String("Pizza?"), Options: []string{"Yes", "No"}},
	}
	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: createInfo, Event: create})

	voteInfo := events.MessageInfo{Sender: voter, ChatID: creator.String()}
	voteTargetBytes, err := creator.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	vote := &signalpb.DataMessage{PollVote: &signalpb.DataMessage_PollVote{
		TargetAuthorAciBinary: voteTargetBytes, TargetSentTimestamp: proto.Uint64(5), OptionIndexes: []uint32{0},
	}}
	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: voteInfo, Event: vote})

	pollID := messageRemoteID(creator, 5)
	poll, ok := sink.PollFor(creator.String(), pollID)
	if !ok || poll.Options[0].Votes != 1 {
		t.Fatalf("poll after vote = %+v, ok=%t", poll, ok)
	}

	terminate := &signalpb.DataMessage{PollTerminate: &signalpb.DataMessage_PollTerminate{TargetSentTimestamp: proto.Uint64(5)}}
	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: createInfo, Event: terminate})

	poll, ok = sink.PollFor(creator.String(), pollID)
	if !ok || !poll.Closed {
		t.Errorf("poll after terminate = %+v, ok=%t, want closed", poll, ok)
	}
}
