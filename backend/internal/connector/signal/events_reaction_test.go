package signal

// events_reaction_test.go covers handleReaction, split out of
// events_test.go to keep that file under the line limit; see its own
// comment.

import (
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"google.golang.org/protobuf/proto"
)

// TestHandleEvent_Reaction checks that a live reaction is reported as
// a reaction chip on the target message, by sender and sent timestamp.
func TestHandleEvent_Reaction(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	reactor := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: reactor, ChatID: author.String()}
	reaction := &signalpb.DataMessage_Reaction{
		Emoji: proto.String("👍"), TargetAuthorAci: proto.String(author.String()), TargetSentTimestamp: proto.Uint64(42),
	}
	msg := &signalpb.DataMessage{Reaction: reaction}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	wantTarget := messageRemoteID(author, 42)
	if !sink.Has("reacted " + author.String() + " " + wantTarget + " 1") {
		t.Errorf("reaction not reported as expected: %v", sink.Lines())
	}
}

// TestHandleReaction_InvalidTargetAuthor checks that a reaction naming
// a malformed target author is dropped rather than panicking.
func TestHandleReaction_InvalidTargetAuthor(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: uuid.New(), ChatID: "x"}
	reaction := &signalpb.DataMessage_Reaction{Emoji: proto.String("👍"), TargetAuthorAci: proto.String("not-a-uuid")}

	c.handleReaction(t.Context(), sink, info, reaction)

	if len(sink.Lines()) != 0 {
		t.Errorf("reaction reported despite an invalid target author: %v", sink.Lines())
	}
}

// TestHandleReaction_Remove checks that a removed reaction clears the
// reactor's own chip.
func TestHandleReaction_Remove(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	reactor := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: reactor, ChatID: author.String()}
	add := &signalpb.DataMessage_Reaction{Emoji: proto.String("👍"), TargetAuthorAci: proto.String(author.String()), TargetSentTimestamp: proto.Uint64(1)}
	c.handleReaction(t.Context(), sink, info, add)

	remove := &signalpb.DataMessage_Reaction{Remove: proto.Bool(true), TargetAuthorAci: proto.String(author.String()), TargetSentTimestamp: proto.Uint64(1)}
	c.handleReaction(t.Context(), sink, info, remove)

	wantTarget := messageRemoteID(author, 1)
	if !sink.Has("reacted " + author.String() + " " + wantTarget + " 0") {
		t.Errorf("reaction not cleared: %v", sink.Lines())
	}
}
