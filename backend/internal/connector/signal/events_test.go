package signal

// events_test.go covers handleEvent's own dispatch and the message
// and edit handling in events.go. Reactions, polls and group/contact
// handling each have enough cases of their own to earn their own file
// (events_reaction_test.go, events_poll_test.go, events_group_test.go),
// all mirroring this same source file.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/libsignalgo"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
)

// upgradeSink wraps connectortest.Sink with a RemoteIDUpgrader, the
// one optional Sink interface the shared test fake does not implement,
// so handleACIFound can be exercised.
type upgradeSink struct {
	*connectortest.Sink
	oldID, newID string
}

func (s *upgradeSink) ConversationRemoteIDChanged(_ context.Context, _, oldRemoteID, newRemoteID string) {
	s.oldID, s.newID = oldRemoteID, newRemoteID
}

// connectedTestConnector returns a connector already "connected" to
// dev, with sink recording everything reported and an in-memory media
// store wired in, so an attachment, poll or message's metadata saved
// during the test can be read back.
func connectedTestConnector(dev *fakeDevice) (*Connector, *connectortest.Sink) {
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	media, err := newInMemoryMediaStore(context.Background())
	if err != nil {
		panic(err) // the in-memory driver cannot fail to open; a test helper has no *testing.T to report through
	}
	c.connected(dev, sink, media)

	return c, sink
}

// TestHandleEvent_TextMessage checks that a live text message is
// reported as Incoming, with the chat reported first as a new
// conversation.
func TestHandleEvent_TextMessage(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.names[sender] = "Alice"
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1700000000000)}

	if ok := c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg}); !ok {
		t.Fatal("handleEvent() = false, want true")
	}

	if conv, ok := sink.ConversationFor(sender.String()); !ok || conv.Title != "Alice" {
		t.Errorf("conversation = %+v, ok=%t, want titled Alice", conv, ok)
	}
	live := sink.LiveMessages()[sender.String()]
	if len(live) != 1 || live[0].Text != "hi" || live[0].SenderName != "Alice" {
		t.Errorf("live messages = %+v", live)
	}
}

// TestHandleEvent_Delete checks that a live delete reports the
// message gone, targeting the deleting sender's own earlier message.
func TestHandleEvent_Delete(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	del := &signalpb.DataMessage_Delete{TargetSentTimestamp: proto.Uint64(99)}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: &signalpb.DataMessage{Delete: del}})

	wantID := messageRemoteID(sender, 99)
	if !sink.Has("deleted " + sender.String() + " " + wantID) {
		t.Errorf("delete not recorded for %q: %v", wantID, sink.Lines())
	}
}

// TestHandleEvent_Typing checks that a typing indicator starting and
// stopping both reach the sink.
func TestHandleEvent_Typing(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.names[sender] = "Alice"
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	started := &signalpb.TypingMessage{Action: signalpb.TypingMessage_STARTED.Enum()}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: started})

	if !sink.Has("typing " + sender.String() + " true") {
		t.Errorf("typing start not reported: %v", sink.Lines())
	}
}

// TestHandleEvent_Edit checks that a live edit reports the updated
// text against the original message's remote id.
func TestHandleEvent_Edit(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	edit := &signalpb.EditMessage{
		TargetSentTimestamp: proto.Uint64(10),
		DataMessage:         &signalpb.DataMessage{Body: proto.String("edited"), Timestamp: proto.Uint64(10)},
	}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: edit})

	wantID := messageRemoteID(sender, 10)
	if !sink.Has("edited " + sender.String() + " " + wantID) {
		t.Errorf("edit not reported for %q: %v", wantID, sink.Lines())
	}
}

// TestHandleMessage_Outgoing checks that a synced outgoing message is
// reported through History rather than Incoming.
func TestHandleMessage_Outgoing(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("sent from another device"), Timestamp: proto.Uint64(1)}

	c.handleMessage(t.Context(), sink, info, msg, true)

	history := sink.Messages()[sender.String()]
	if len(history) != 1 || !history[0].Outgoing {
		t.Errorf("history = %+v, want one outgoing message", history)
	}
}

// TestHandleMessage_SavesIncomingMessageMeta checks that an incoming
// message's server GUID and report-spam token are saved for later
// use, while a synced outgoing message, which carries neither for
// itself, saves nothing.
func TestHandleMessage_SavesIncomingMessageMeta(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String(), ServerGUID: "guid-1", ReportingToken: []byte{1, 2}}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(5)}
	c.handleMessage(t.Context(), sink, info, msg, false)

	wantID := messageRemoteID(sender, 5)
	guid, token, found, err := c.mediaFor().messageMeta(t.Context(), sender.String(), wantID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || guid != "guid-1" || string(token) != "\x01\x02" {
		t.Errorf("messageMeta() = %q %v found=%t", guid, token, found)
	}

	outgoingInfo := events.MessageInfo{Sender: sender, ChatID: sender.String(), ServerGUID: "should-not-save"}
	outgoingMsg := &signalpb.DataMessage{Body: proto.String("from another device"), Timestamp: proto.Uint64(6)}
	c.handleMessage(t.Context(), sink, outgoingInfo, outgoingMsg, true)

	_, _, found, err = c.mediaFor().messageMeta(t.Context(), sender.String(), messageRemoteID(sender, 6))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("messageMeta() saved metadata for a synced outgoing message")
	}
}

// TestHandleMessage_Outgoing_StoresTheMessage drives the same synced
// outgoing message through a real app.Ingest over a real store: a
// message sent from the user's own phone goes through History rather
// than Incoming, a different path through Ingest.save, so it earns
// its own end-to-end check rather than relying on
// TestHandleMessage_Outgoing's sink-only recording.
func TestHandleMessage_Outgoing_StoresTheMessage(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	ingest := newRealIngest(t)
	media := mustOpenMediaStore(t)
	c.connected(dev, ingest, media)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	c.ensureConversation(t.Context(), ingest, info) // handleDataMessage's own precondition for handleMessage
	msg := &signalpb.DataMessage{Body: proto.String("sent from another device"), Timestamp: proto.Uint64(1)}
	c.handleMessage(t.Context(), ingest, info, msg, true)

	conv, err := ingest.store.ConversationByRemote(t.Context(), "acc1", sender.String())
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}

	messages, _, err := ingest.store.Messages(t.Context(), conv.ID, "", 10)
	if err != nil || len(messages) != 1 || !messages[0].Outgoing {
		t.Fatalf("messages = %+v, %v; want one outgoing message stored", messages, err)
	}
}

// TestHandleMessage_BeforeConversationReported_DropsAndLogs checks
// that a message reported before its conversation - never possible
// through handleDataMessage's own ensureConversation call, but
// reachable if a future caller skips it, the way this test does on
// purpose - is dropped by the real store rather than stored
// unreachably, and that the drop is logged rather than silent.
func TestHandleMessage_BeforeConversationReported_DropsAndLogs(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	ingest := newRealIngest(t)
	media := mustOpenMediaStore(t)
	c.connected(dev, ingest, media)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}
	c.handleMessage(t.Context(), ingest, info, msg, false)

	if _, err := ingest.store.ConversationByRemote(t.Context(), "acc1", sender.String()); err == nil {
		t.Fatal("ConversationByRemote() succeeded for a conversation never reported")
	}
	if lines := ingest.logger.take(); len(lines) != 1 {
		t.Errorf("logged lines = %v, want exactly one drop logged", lines)
	}
}

// TestHandleEdit_NoDataMessage checks that an edit carrying no data
// message is dropped rather than reporting an empty message.
func TestHandleEdit_NoDataMessage(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: uuid.New(), ChatID: "x"}
	c.handleEdit(t.Context(), sink, info, &signalpb.EditMessage{TargetSentTimestamp: proto.Uint64(1)})

	if len(sink.Lines()) != 0 {
		t.Errorf("edit reported with no data message: %v", sink.Lines())
	}
}

// TestHandleEvent_ACIFound checks that learning a contact's real
// account identifier upgrades the conversation's remote id.
func TestHandleEvent_ACIFound(t *testing.T) {
	t.Parallel()

	pni := uuid.New()
	aci := uuid.New()
	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)
	sink := &upgradeSink{Sink: &connectortest.Sink{}}

	found := &events.ACIFound{PNI: libsignalgo.NewPNIServiceID(pni), ACI: libsignalgo.NewACIServiceID(aci)}
	c.handleEvent(t.Context(), sink, found)

	if sink.oldID != pni.String() || sink.newID != aci.String() {
		t.Errorf("oldID=%q newID=%q, want %q %q", sink.oldID, sink.newID, pni, aci)
	}
}

// TestHandleEvent_Unhandled checks that an event kind this connector
// does not act on is still reported handled, so signalmeow's own
// retry machinery never blocks on it.
func TestHandleEvent_Unhandled(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice(uuid.New()))

	if ok := c.handleEvent(t.Context(), &connectortest.Sink{}, &events.LoggedOut{}); !ok {
		t.Error("handleEvent() = false for an unhandled event, want true")
	}
}
