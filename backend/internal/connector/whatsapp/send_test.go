package whatsapp

// Send is driven through a fake device, as the pairing tests are,
// because the fake is the only way to see what Connector asked
// whatsmeow to send without reaching WhatsApp's servers.

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// directPeer and directChat are a one-to-one chat this package's send
// and receipt tests send to.
var (
	directPeer = types.NewJID("15551234567", types.DefaultUserServer)
	directChat = domain.Conversation{RemoteID: remoteID(directPeer), Kind: domain.KindDirect}
)

// groupJID and groupChat are a group chat with two other members besides
// the signed-in account, for the tests that check group delivery
// progress waits for both of them.
var (
	groupJID  = types.NewJID("120036304151000", types.GroupServer)
	groupChat = domain.Conversation{RemoteID: remoteID(groupJID), Kind: domain.KindGroup, Members: 3}
)

func TestSend_SendsPlainTextAndReportsItSent(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.nextMessageID = "wire-1"
	var sink connectortest.Sink

	c := connectedTo(dev, &sink)
	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	if len(dev.sent) != 1 || dev.sent[0].jid != directPeer || dev.sent[0].msg.GetConversation() != "hi" {
		t.Fatalf("sent = %+v, want one text message to the peer", dev.sent)
	}

	if !sink.Has("outgoing local-1 wire-1 " + domain.StatusSent) {
		t.Errorf("events = %q, want local-1 reported sent with its wire id", sink.Lines())
	}
}

func TestSend_QuotesAReplyByStanzaIDAlone(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink

	c := connectedTo(dev, &sink)
	m := domain.Message{ID: "local-2", Text: "sure", ReplyTo: &domain.Reply{RemoteID: "quoted-1", SenderName: "Nadia"}}
	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	msg := dev.sent[0].msg
	ctx := msg.GetExtendedTextMessage().GetContextInfo()
	if msg.GetExtendedTextMessage().GetText() != "sure" || ctx.GetStanzaID() != "quoted-1" {
		t.Fatalf("message = %+v, want an extended text message quoting quoted-1", msg)
	}
	if ctx.GetParticipant() != "" || ctx.GetQuotedMessage() != nil {
		t.Errorf("context = %+v, want no participant or quoted message, which the store does not keep", ctx)
	}
}

func TestSend_RejectsAnAttachment(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	m := domain.Message{ID: "local-3", Media: &domain.Media{Kind: domain.MediaPhoto}}
	if err := c.Send(t.Context(), directChat, m); !errors.Is(err, errMediaNotSupported) {
		t.Errorf("Send = %v, want errMediaNotSupported", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent for an unsupported attachment", dev.sent)
	}
}

func TestSend_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	err := c.Send(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}, domain.Message{ID: "local-4"})
	if !errors.Is(err, errBadRemoteID) {
		t.Errorf("Send = %v, want errBadRemoteID", err)
	}
}

func TestSend_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.sendErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-5"}); !errors.Is(err, dev.sendErr) {
		t.Errorf("Send = %v, want it to wrap the device's error", err)
	}
	if len(sink.Outgoing("local-5")) != 0 {
		t.Errorf("outgoing updates = %v, want none for a failed send", sink.Outgoing("local-5"))
	}
}

func TestSend_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.sendBlocks = true
		var sink connectortest.Sink
		c := connectedTo(dev, &sink)

		done := make(chan error, 1)
		go func() { done <- c.Send(t.Context(), directChat, domain.Message{ID: "local-6"}) }()

		time.Sleep(sendTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Send = %v, want context.DeadlineExceeded after the send timeout", err)
			}
		default:
			t.Fatal("Send did not return once its timeout elapsed")
		}
	})
}
