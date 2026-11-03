package store_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAddMessage_RoundTripsSenderVerified confirms a message's
// SenderVerified is stored and read back exactly as given, both true
// and false.
func TestAddMessage_RoundTripsSenderVerified(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s,
		domain.Message{ID: "verified", ConversationID: "chat", RemoteID: "1", SenderName: "Alex", SenderVerified: true, Text: "hi", Created: 1},
		domain.Message{ID: "unverified", ConversationID: "chat", RemoteID: "2", SenderName: "Mum", SenderVerified: false, Text: "it's me", Created: 2},
	)

	verified, err := s.Message(ctx, "verified")
	if err != nil || !verified.SenderVerified {
		t.Errorf("verified message.SenderVerified = %t, %v, want true", verified.SenderVerified, err)
	}

	unverified, err := s.Message(ctx, "unverified")
	if err != nil || unverified.SenderVerified {
		t.Errorf("unverified message.SenderVerified = %t, %v, want false", unverified.SenderVerified, err)
	}
}

// TestAddMessage_FillsReplyVerifiedFromTheQuotedMessage confirms a
// reply quote's own SenderVerified is resolved from the quoted
// message the same way its SenderName and SenderID already are, so
// the UI can tell a quote of a self-chosen, unverified name apart from
// one of a saved contact's (see docs/decisions.md).
func TestAddMessage_FillsReplyVerifiedFromTheQuotedMessage(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{
		ID: "quoted", ConversationID: "chat", RemoteID: "1",
		SenderID: "r-mallory", Text: "it's me, Mum", SenderName: "Mum", SenderVerified: false, Created: 1,
	})

	reply, _, err := s.AddMessage(ctx, domain.Message{
		ID: "reply", ConversationID: "chat", RemoteID: "2", Text: "ok", Created: 2,
		ReplyTo: &domain.Reply{RemoteID: "1"},
	})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.SenderVerified {
		t.Fatalf("reply.ReplyTo = %+v, want SenderVerified false, copied from the quoted message", reply.ReplyTo)
	}

	stored, err := s.Message(ctx, "reply")
	if err != nil || stored.ReplyTo == nil || stored.ReplyTo.SenderVerified {
		t.Errorf("stored reply = %+v, %v, want SenderVerified false", stored.ReplyTo, err)
	}
}
