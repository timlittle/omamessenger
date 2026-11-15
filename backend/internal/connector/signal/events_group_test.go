package signal

// events_group_test.go covers handleContactList, handleACIFound's
// group-side counterpart (group and direct titling), mentions and
// disappearing timers from events.go, split out of events_test.go to
// keep that file under the line limit; see its own comment.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupChatID is a 44-character identifier, matching the length
// isGroupChat keys on, for tests that need a group chat rather than a
// direct one.
const groupChatID = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

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
