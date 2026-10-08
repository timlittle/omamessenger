package whatsapp

// Outgoing receipt progress is driven through a fake device, by firing
// the event fakeDevice.onEvent recorded, since that is the only way to
// exercise it without reaching WhatsApp. MarkRead itself is covered in
// markread_test.go.

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupMemberA and groupMemberB are the two other members of groupChat
// (see send_test.go), whose receipts must both arrive before a group
// message's tick advances.
var (
	groupMemberA = types.NewJID("15551111111", types.DefaultUserServer)
	groupMemberB = types.NewJID("15552222222", types.DefaultUserServer)
)

func TestReceipt_DirectChatReportsOnTheOnlyRecipient(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

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

	dev, sink, c := connectedFixture(t)
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("111222", types.HiddenUserServer)
	dev.selfJID = phone
	dev.selfLID = lid

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

	dev, sink, c := connectedFixture(t)

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

// TestReceipt_GroupDoesNotDoubleCountTheSameMemberUnderTwoAddressForms
// reproduces the bug fixed by canonicalizing receipt's participant key
// (see personID in normalize.go): the same real group member reporting
// delivery under their LID and then again under their mapped phone JID
// must count as one participant, not two, so the group's tick does not
// advance as "delivered" before the other member has reported at all.
func TestReceipt_GroupDoesNotDoubleCountTheSameMemberUnderTwoAddressForms(t *testing.T) {
	t.Parallel()

	lidForA := types.NewJID("999888", types.HiddenUserServer)
	dev, sink, c := connectedFixture(t)
	dev.lidPhones = map[string]types.JID{lidForA.String(): groupMemberA}

	if err := c.Send(t.Context(), groupChat, domain.Message{ID: "local-4"}); err != nil {
		t.Fatal(err)
	}
	wireID := dev.sent[0].id

	// The same member, A, reports delivery twice, once under each of
	// their two address forms.
	c.fireReceipt(t.Context(), groupJID, lidForA, wireID, types.ReceiptTypeDelivered)
	c.fireReceipt(t.Context(), groupJID, groupMemberA, wireID, types.ReceiptTypeDelivered)
	if sink.Has("outgoing local-4  " + domain.StatusDelivered) {
		t.Fatal("local-4 reported delivered after only one real member (counted twice) reported, want it to wait for the other member")
	}

	c.fireReceipt(t.Context(), groupJID, groupMemberB, wireID, types.ReceiptTypeDelivered)
	if !sink.Has("outgoing local-4  " + domain.StatusDelivered) {
		t.Errorf("events = %q, want local-4 delivered once both distinct members have reported", sink.Lines())
	}
}

func TestReceipt_IgnoresAMessageThisAccountDidNotSend(t *testing.T) {
	t.Parallel()

	_, sink, c := connectedFixture(t)

	c.fireReceipt(t.Context(), directPeer, directPeer, "unknown-wire-id", types.ReceiptTypeRead)

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for an untracked message", sink.Lines())
	}
}

func TestReceipt_IgnoresAReceiptKindItDoesNotTrack(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

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
	c.receipt(ctx, c.sink, c.dev, c.media, &events.Receipt{
		MessageSource: types.MessageSource{Chat: chat, Sender: participant, IsGroup: chat.Server == types.GroupServer},
		Type:          rt,
		MessageIDs:    []types.MessageID{id},
	})
}
