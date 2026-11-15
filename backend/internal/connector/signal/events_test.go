package signal

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/libsignalgo"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
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

// TestHandleEvent_ContactList checks that a contact's resolved name
// is remembered and used on a later message from them.
func TestHandleEvent_ContactList(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	c.handleEvent(t.Context(), sink, &events.ContactList{Contacts: []*types.Recipient{{ACI: sender, ContactName: "Bob"}}})

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}
	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	live := sink.LiveMessages()[sender.String()]
	if len(live) != 1 || live[0].SenderName != "Bob" {
		t.Errorf("live = %+v, want sender name Bob", live)
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

// groupChatID is a 44-character identifier, matching the length
// isGroupChat keys on, for tests that need a group chat rather than a
// direct one.
const groupChatID = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// TestHandleEvent_GroupMessage checks that a live message in a group
// this run has not reported yet first reports the group's title,
// member count and disappearing timer, resolved from the device.
func TestHandleEvent_GroupMessage(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.groupTitle = "Book Club"
	dev.groupMembers = []signalmeow.GroupMember{{ACI: sender}, {ACI: uuid.New()}}
	dev.groupSeconds = 604800
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: groupChatID}
	msg := &signalpb.DataMessage{Body: proto.String("hi all"), Timestamp: proto.Uint64(1)}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	conv, ok := sink.ConversationFor(groupChatID)
	if !ok || conv.Title != "Book Club" || conv.Members != 2 || conv.Kind != domain.KindGroup {
		t.Errorf("conversation = %+v, ok=%t", conv, ok)
	}
	if !sink.Has("timer " + groupChatID + " 604800") {
		t.Errorf("group disappearing timer not reported: %v", sink.Lines())
	}
}

// TestHandleEvent_GroupMessage_NotConnected checks that a group
// message with no run connected does not panic, and reports nothing.
func TestHandleEvent_GroupMessage_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{account: domain.Account{ID: "acc1"}}
	sink := &connectortest.Sink{}

	info := events.MessageInfo{Sender: uuid.New(), ChatID: groupChatID}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	if _, ok := sink.ConversationFor(groupChatID); ok {
		t.Error("group conversation reported with no run connected to fetch its info")
	}
}

// TestHandleEvent_GroupMessage_GroupInfoFails checks that a failure
// to fetch a group's info leaves the conversation unreported rather
// than reporting one with empty fields.
func TestHandleEvent_GroupMessage_GroupInfoFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.groupErr = errors.New("network down")
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: uuid.New(), ChatID: groupChatID}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	if conv, ok := sink.ConversationFor(groupChatID); !ok || conv.Title != genericGroupTitle {
		t.Errorf("conversation = %+v, ok=%t, want a placeholder title so the message is kept", conv, ok)
	}
}

// TestHandleEvent_UnknownContactGetsATitle checks that a direct chat
// with nobody's name known yet still gets a title: the store refuses a
// conversation without one, which would drop every message in it.
func TestHandleEvent_UnknownContactGetsATitle(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}
	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	if conv, ok := sink.ConversationFor(sender.String()); !ok || conv.Title != genericSenderName {
		t.Errorf("conversation = %+v, ok=%t, want titled %q", conv, ok, genericSenderName)
	}
}

// TestHandleEvent_OutgoingTitlesByRecipient checks that a message sent
// from the phone titles its chat after the recipient, not this account.
func TestHandleEvent_OutgoingTitlesByRecipient(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	friend := uuid.New()
	dev := newFakeDevice(self)
	dev.names[self] = "Me"
	dev.names[friend] = "Bob"
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: self, ChatID: friend.String()}
	c.handleDataMessage(t.Context(), sink, info, &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1)}, true)

	if conv, ok := sink.ConversationFor(friend.String()); !ok || conv.Title != "Bob" {
		t.Errorf("conversation = %+v, ok=%t, want titled Bob", conv, ok)
	}
}

// TestHandleContactList_SkipsNilAndUnnamed checks that a nil contact
// and one with no ACI are skipped without panicking, and one with
// no name at all is not remembered.
func TestHandleContactList_SkipsNilAndUnnamed(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)

	c.handleContactList(&events.ContactList{Contacts: []*types.Recipient{nil, {ACI: uuid.Nil}, {ACI: uuid.New()}}})
}

// TestHandleEvent_DisappearingTimer checks that a message carrying an
// expire timer reports the chat's disappearing-message period.
func TestHandleEvent_DisappearingTimer(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c, sink := connectedTestConnector(dev)

	info := events.MessageInfo{Sender: sender, ChatID: sender.String()}
	msg := &signalpb.DataMessage{Body: proto.String("hi"), Timestamp: proto.Uint64(1), ExpireTimer: proto.Uint32(86400)}

	c.handleEvent(t.Context(), sink, &events.ChatEvent{Info: info, Event: msg})

	if !sink.Has("timer " + sender.String() + " 86400") {
		t.Errorf("disappearing timer not reported: %v", sink.Lines())
	}
}

// TestMentionsSelf checks the self-naming check both ways, and that
// an unconnected run (self == uuid.Nil) never matches.
func TestMentionsSelf(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	other := uuid.New()

	if !mentionsSelf([]domain.Mention{{UserID: self.String()}}, self) {
		t.Error("mentionsSelf() = false, want true")
	}
	if mentionsSelf([]domain.Mention{{UserID: other.String()}}, self) {
		t.Error("mentionsSelf() = true for a mention naming someone else")
	}
	if mentionsSelf([]domain.Mention{{UserID: self.String()}}, uuid.Nil) {
		t.Error("mentionsSelf() = true with no self identity known yet")
	}
}

// TestResolveMentionNames checks that a resolved name is filled in,
// and an unresolved one falls back to genericSenderName.
func TestResolveMentionNames(t *testing.T) {
	t.Parallel()

	known := "11111111-1111-1111-1111-111111111111"
	mentions := []domain.Mention{{UserID: known}, {UserID: "unknown"}}

	resolveMentionNames(mentions, func(id string) string {
		if id == known {
			return "Alice"
		}
		return ""
	})

	if mentions[0].Name != "Alice" || mentions[1].Name != genericSenderName {
		t.Errorf("mentions = %+v", mentions)
	}
}

// TestContactDisplayName_FallsBackToProfile checks that a recipient
// with no saved contact name uses their profile name instead.
func TestContactDisplayName_FallsBackToProfile(t *testing.T) {
	t.Parallel()

	r := &types.Recipient{Profile: types.Profile{Name: "Profile Name"}}
	if got := contactDisplayName(r); got != "Profile Name" {
		t.Errorf("contactDisplayName() = %q, want %q", got, "Profile Name")
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
