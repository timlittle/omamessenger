package whatsapp

// MarkRead and receipt progress are driven through a fake device and,
// for receipts, by firing the event fakeDevice.onEvent recorded, since
// neither has any other way to be exercised without reaching WhatsApp.

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupMemberA and groupMemberB are the two other members of groupChat
// (see send_test.go), whose receipts must both arrive before a group
// message's tick advances.
var (
	groupMemberA = types.NewJID("15551111111", types.DefaultUserServer)
	groupMemberB = types.NewJID("15552222222", types.DefaultUserServer)
)

func TestMarkRead_SendsOneCallPerSenderThenForgetsThem(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	c.notePendingRead(groupChat.RemoteID, remoteID(groupMemberA), "m1")
	c.notePendingRead(groupChat.RemoteID, remoteID(groupMemberA), "m2")
	c.notePendingRead(groupChat.RemoteID, remoteID(groupMemberB), "m3")

	if err := c.MarkRead(t.Context(), groupChat); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 2 {
		t.Fatalf("markRead calls = %d, want 2, one per sender: %+v", len(dev.markReadCalls), dev.markReadCalls)
	}

	byChat := groupJID
	for _, call := range dev.markReadCalls {
		if call.chat != byChat {
			t.Errorf("call chat = %v, want the group JID", call.chat)
		}
	}

	if err := c.MarkRead(t.Context(), groupChat); err != nil {
		t.Fatal(err)
	}
	if len(dev.markReadCalls) != 2 {
		t.Errorf("markRead calls after a second MarkRead = %d, want still 2: nothing left pending", len(dev.markReadCalls))
	}
}

func TestMarkRead_NoopWithNothingPending(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.MarkRead(t.Context(), directChat); err != nil {
		t.Fatal(err)
	}
	if len(dev.markReadCalls) != 0 {
		t.Errorf("markRead calls = %d, want none", len(dev.markReadCalls))
	}
}

func TestMarkRead_FailsBeforeConnecting(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice())
	if err := c.MarkRead(t.Context(), directChat); !errors.Is(err, errNotConnected) {
		t.Errorf("MarkRead = %v, want errNotConnected", err)
	}
}

func TestMarkRead_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	c.notePendingRead("not-a-jid", remoteID(directPeer), "m1")
	if err := c.MarkRead(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}); !errors.Is(err, errBadRemoteID) {
		t.Errorf("MarkRead = %v, want errBadRemoteID", err)
	}
}

func TestReceipt_DirectChatReportsOnTheOnlyRecipient(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-1"}); err != nil {
		t.Fatal(err)
	}
	wireID := dev.sent[0].id

	c.fireReceipt(t.Context(), directPeer, directPeer, wireID, types.ReceiptTypeDelivered)
	if !sink.Has("outgoing local-1  " + domain.StatusDelivered) {
		t.Errorf("events = %q, want local-1 delivered", sink.Lines())
	}

	c.fireReceipt(t.Context(), directPeer, directPeer, wireID, types.ReceiptTypeRead)
	if !sink.Has("outgoing local-1  " + domain.StatusRead) {
		t.Errorf("events = %q, want local-1 read", sink.Lines())
	}
}

func TestReceipt_CanonicalizesTheSelfChatsLIDToMatchTheSentMessage(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("111222", types.HiddenUserServer)
	dev.selfJID = phone
	dev.selfLID = lid
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	selfChat := domain.Conversation{RemoteID: remoteID(phone), Kind: domain.KindDirect}
	if err := c.Send(t.Context(), selfChat, domain.Message{ID: "local-self"}); err != nil {
		t.Fatal(err)
	}
	wireID := dev.sent[0].id

	// WhatsApp reports the receipt addressed by the account's LID, not
	// the phone JID Send used.
	c.fireReceipt(t.Context(), lid, lid, wireID, types.ReceiptTypeDelivered)

	if !sink.Has("outgoing local-self  " + domain.StatusDelivered) {
		t.Errorf("events = %q, want the LID-addressed receipt matched back to the phone-addressed send", sink.Lines())
	}
}

func TestReceipt_GroupWaitsForEveryMemberBeforeAdvancing(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.Send(t.Context(), groupChat, domain.Message{ID: "local-2"}); err != nil {
		t.Fatal(err)
	}
	wireID := dev.sent[0].id

	c.fireReceipt(t.Context(), groupJID, groupMemberA, wireID, types.ReceiptTypeDelivered)
	if sink.Has("outgoing local-2  " + domain.StatusDelivered) {
		t.Fatal("local-2 reported delivered after only one of two members received it")
	}

	c.fireReceipt(t.Context(), groupJID, groupMemberB, wireID, types.ReceiptTypeDelivered)
	if !sink.Has("outgoing local-2  " + domain.StatusDelivered) {
		t.Errorf("events = %q, want local-2 delivered once both members have it", sink.Lines())
	}

	c.fireReceipt(t.Context(), groupJID, groupMemberA, wireID, types.ReceiptTypeRead)
	if sink.Has("outgoing local-2  " + domain.StatusRead) {
		t.Error("local-2 reported read after only one of two members read it")
	}

	c.fireReceipt(t.Context(), groupJID, groupMemberB, wireID, types.ReceiptTypeRead)
	if !sink.Has("outgoing local-2  " + domain.StatusRead) {
		t.Errorf("events = %q, want local-2 read once both members have read it", sink.Lines())
	}
}

func TestReceipt_IgnoresAMessageThisAccountDidNotSend(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	c.fireReceipt(t.Context(), directPeer, directPeer, "unknown-wire-id", types.ReceiptTypeRead)

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for an untracked message", sink.Lines())
	}
}

func TestReceipt_IgnoresAReceiptKindItDoesNotTrack(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-3"}); err != nil {
		t.Fatal(err)
	}

	c.fireReceipt(t.Context(), directPeer, directPeer, dev.sent[0].id, types.ReceiptTypeRetry)
	if updates := sink.Outgoing("local-3"); len(updates) != 1 {
		t.Errorf("outgoing updates = %v, want only the initial sent status, nothing for a retry receipt", updates)
	}
}

// fireReceipt fires a receipt event on c's own sink, as whatsmeow would
// for a receipt naming chat, from participant, for the given WhatsApp
// message id and type.
func (c *Connector) fireReceipt(ctx context.Context, chat, participant types.JID, id types.MessageID, rt types.ReceiptType) {
	c.receipt(ctx, c.sink, c.dev, &events.Receipt{
		MessageSource: types.MessageSource{Chat: chat, Sender: participant, IsGroup: chat.Server == types.GroupServer},
		Type:          rt,
		MessageIDs:    []types.MessageID{id},
	})
}
