package store_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAddMessage_ArmsExpiryImmediatelyUnlessItStartsOnRead checks the
// two timer rules AddMessage itself can resolve: a period counted from
// the message's own created time (WhatsApp and Telegram) is armed at
// once, while one that starts only once the message is read (Signal's
// own rule for something received) is left unarmed until then.
func TestAddMessage_ArmsExpiryImmediatelyUnlessItStartsOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	armed, _, err := s.AddMessage(ctx, domain.Message{
		ID: "a", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1000, ExpireAfter: 10,
	})
	if err != nil || armed.ExpiresAt != 1000+10*1000 {
		t.Fatalf("armed.ExpiresAt = %d, %v, want %d", armed.ExpiresAt, err, 1000+10*1000)
	}

	notYet, _, err := s.AddMessage(ctx, domain.Message{
		ID: "b", ConversationID: "chat", RemoteID: "r2", Text: "hi", Created: 1000, ExpireAfter: 10, ArmOnRead: true,
	})
	if err != nil || notYet.ExpiresAt != 0 {
		t.Fatalf("notYet.ExpiresAt = %d, %v, want 0 until read", notYet.ExpiresAt, err)
	}
}

// TestMarkRead_ArmsAnExpiryThatStartsOnRead checks that marking a
// conversation read arms the expiry of a message in it whose timer
// starts only then, never before and never a second time.
func TestMarkRead_ArmsAnExpiryThatStartsOnRead(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if _, _, err := s.AddMessage(ctx, domain.Message{
		ID: "a", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1000, ExpireAfter: 10, ArmOnRead: true,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	if _, err := s.MarkRead(ctx, "chat"); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	read, err := s.Message(ctx, "a")
	if err != nil || read.ExpiresAt == 0 {
		t.Fatalf("read = %+v, %v, want an armed expiry", read, err)
	}

	if _, err := s.MarkRead(ctx, "chat"); err != nil {
		t.Fatalf("mark read again: %v", err)
	}

	again, err := s.Message(ctx, "a")
	if err != nil || again.ExpiresAt != read.ExpiresAt {
		t.Fatalf("again = %+v, %v, want the same already-armed expiry, unchanged", again, err)
	}
}

// TestAddHistoryMessage_DropsOneAlreadyPastItsExpiry checks that
// backfilling history never resurrects a disappearing message whose
// timer, armed immediately, has already run out.
func TestAddHistoryMessage_DropsOneAlreadyPastItsExpiry(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	_, inserted, err := s.AddHistoryMessage(ctx, domain.Message{
		ID: "old", ConversationID: "chat", RemoteID: "r1", Text: "gone", Created: 1, ExpireAfter: 1,
	})
	if err != nil || inserted {
		t.Fatalf("inserted = %t, %v, want an already-expired history message dropped", inserted, err)
	}

	if _, err := s.Message(ctx, "old"); err == nil {
		t.Error("an already-expired history message was stored")
	}
}

// TestPendingExpiries_OrdersBySoonestDue checks the sweeper's own
// lookup: only armed expiries are listed, soonest first.
func TestPendingExpiries_OrdersBySoonestDue(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	addMessages(t, s,
		domain.Message{ID: "none", ConversationID: "chat", RemoteID: "r0", Text: "hi", Created: 1000},
		domain.Message{ID: "later", ConversationID: "chat", RemoteID: "r1", Text: "hi", Created: 1000, ExpireAfter: 100},
		domain.Message{ID: "sooner", ConversationID: "chat", RemoteID: "r2", Text: "hi", Created: 1000, ExpireAfter: 10},
	)

	pending, err := s.PendingExpiries(ctx)
	if err != nil || len(pending) != 2 || pending[0].ID != "sooner" || pending[1].ID != "later" {
		t.Fatalf("pending = %+v, %v, want sooner then later", pending, err)
	}
}
