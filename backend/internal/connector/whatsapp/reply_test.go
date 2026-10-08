package whatsapp

// reply_test.go checks that Send renders a proper quote once the quoted
// message's sender is known locally, and still falls back to the
// stanza id alone when it is not; TestSend_QuotesAReplyByStanzaIDAlone
// in send_test.go already covers the plain fallback with no media store
// at all.

import (
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSend_QuotesWithParticipantAndTextInAGroup(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)

	sender := types.NewJID("15557654321", types.DefaultUserServer)
	if err := media.putMessageKey(t.Context(), groupChat.RemoteID, "quoted-1", messageKey{senderID: remoteID(sender), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	m := domain.Message{ID: "local-1", Text: "sure", ReplyTo: &domain.Reply{RemoteID: "quoted-1", SenderName: "Nadia", Text: "see you at six"}}
	if err := c.Send(t.Context(), groupChat, m); err != nil {
		t.Fatal(err)
	}

	ctx := dev.sent[0].msg.GetExtendedTextMessage().GetContextInfo()
	if ctx.GetStanzaID() != "quoted-1" {
		t.Fatalf("stanza id = %q, want quoted-1", ctx.GetStanzaID())
	}
	if ctx.GetParticipant() != remoteID(sender) {
		t.Errorf("participant = %q, want the quoted message's sender", ctx.GetParticipant())
	}
	if ctx.GetQuotedMessage().GetConversation() != "see you at six" {
		t.Errorf("quoted message = %q, want the reply's stored text", ctx.GetQuotedMessage().GetConversation())
	}
}

func TestSend_QuotesInADirectChatWithoutAParticipant(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "quoted-2", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	m := domain.Message{ID: "local-2", Text: "sure", ReplyTo: &domain.Reply{RemoteID: "quoted-2", Text: "lunch?"}}
	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	ctx := dev.sent[0].msg.GetExtendedTextMessage().GetContextInfo()
	if ctx.GetParticipant() != "" {
		t.Errorf("participant = %q, want none in a direct chat", ctx.GetParticipant())
	}
	if ctx.GetQuotedMessage().GetConversation() != "lunch?" {
		t.Errorf("quoted message = %q, want the reply's stored text", ctx.GetQuotedMessage().GetConversation())
	}
}

func TestSend_QuotesByStanzaIDAloneWhenTheQuoteIsNotRecorded(t *testing.T) {
	t.Parallel()

	dev, _, c, _ := connectedMediaFixture(t)

	m := domain.Message{ID: "local-3", Text: "sure", ReplyTo: &domain.Reply{RemoteID: "quoted-3", Text: "lunch?"}}
	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	ctx := dev.sent[0].msg.GetExtendedTextMessage().GetContextInfo()
	if ctx.GetStanzaID() != "quoted-3" {
		t.Fatalf("stanza id = %q, want quoted-3", ctx.GetStanzaID())
	}
	if ctx.GetParticipant() != "" || ctx.GetQuotedMessage() != nil {
		t.Errorf("context = %+v, want no participant or quoted message for an unrecorded quote", ctx)
	}
}

func TestSend_RecordsItsOwnMessageKeyAsFromMe(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.nextMessageID = "wire-1"

	if err := c.Send(t.Context(), directChat, domain.Message{ID: "local-4", Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	key, found, err := media.messageKeyFor(t.Context(), directChat.RemoteID, "wire-1")
	if err != nil || !found || !key.fromMe {
		t.Errorf("messageKeyFor = %+v found=%v err=%v, want a fromMe key saved for the sent message", key, found, err)
	}
}
