package signal

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestVote_SendsVoteAndReportsTally checks that Vote sends Signal's
// own PollVote with an increasing per-voter counter and reports the
// poll's own tally once this run tracked it created.
func TestVote_SendsVoteAndReportsTally(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	author := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	media := mustOpenMediaStore(t)
	c.connected(dev, sink, media)

	conv := domain.Conversation{RemoteID: author.String()}
	pollID := messageRemoteID(author, uint64(time.Now().UnixMilli()))
	c.trackPoll(t.Context(), conv.RemoteID, pollID, newPoll{question: "Pizza?", options: []string{"Yes", "No"}})

	if err := c.Vote(t.Context(), conv, pollID, []string{"a"}); err != nil {
		t.Fatalf("Vote() error = %v", err)
	}
	if err := c.Vote(t.Context(), conv, pollID, []string{"b"}); err != nil {
		t.Fatalf("second Vote() error = %v", err)
	}

	if len(dev.sentDirect) != 2 {
		t.Fatalf("sentDirect = %d, want 2", len(dev.sentDirect))
	}
	first := dev.sentDirect[0].content.GetDataMessage().GetPollVote()
	second := dev.sentDirect[1].content.GetDataMessage().GetPollVote()
	if first.GetVoteCount() != 1 || second.GetVoteCount() != 2 {
		t.Errorf("vote counts = %d, %d, want 1, 2", first.GetVoteCount(), second.GetVoteCount())
	}
	if len(second.GetOptionIndexes()) != 1 || second.GetOptionIndexes()[0] != 1 {
		t.Errorf("second vote option indexes = %v, want [1]", second.GetOptionIndexes())
	}

	poll, found := sink.PollFor(conv.RemoteID, pollID)
	if !found || poll.Options[1].Votes != 1 {
		t.Errorf("poll tally = %+v, found=%t, want option b voted once", poll, found)
	}
}

// TestVote_RejectsUnknownOption checks that an option id outside the
// lowercase-letter range formatOptionID produces is refused.
func TestVote_RejectsUnknownOption(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: uuid.New().String()}
	target := messageRemoteID(uuid.New(), uint64(time.Now().UnixMilli()))
	err := c.Vote(t.Context(), conv, target, []string{"not-an-option"})
	if !errors.Is(err, errUnknownPollOption) {
		t.Errorf("Vote() error = %v, want errUnknownPollOption", err)
	}
}

// TestVote_NotConnected checks that nothing connected reports
// errNotConnected rather than panicking.
func TestVote_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.Vote(t.Context(), domain.Conversation{}, "m1", nil); !errors.Is(err, errNotConnected) {
		t.Errorf("Vote() error = %v, want errNotConnected", err)
	}
}

// TestVote_NoMediaStore checks a connector built without a media
// store reports errNotConnected rather than panicking.
func TestVote_NoMediaStore(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	if err := c.Vote(t.Context(), domain.Conversation{}, "m1", nil); !errors.Is(err, errNotConnected) {
		t.Errorf("Vote() error = %v, want errNotConnected", err)
	}
}

// TestVote_RejectsMalformedMessageID checks that a message id that
// does not parse is refused with an error.
func TestVote_RejectsMalformedMessageID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.Vote(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, "not-a-message-id", []string{"a"}); err == nil {
		t.Error("Vote() with a malformed message id succeeded, want an error")
	}
}

// TestVote_DeviceErrorPropagates checks that a failure sending the
// vote is returned rather than silently ignored.
func TestVote_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = errors.New("send failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: uuid.New().String()}
	target := messageRemoteID(uuid.New(), uint64(time.Now().UnixMilli()))
	if err := c.Vote(t.Context(), conv, target, []string{"a"}); err == nil {
		t.Error("Vote() with a failing send succeeded, want an error")
	}
}
