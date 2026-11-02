package store_test

// messages_fillmedia_test.go covers AddMessage's defence against
// merging a reused remote id's media into a message a different
// sender or direction already owns (see messages.go's sameSender),
// kept separate from messages_test.go's own happy-path tests to stay
// within this codebase's file-length limit.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAddMessage_RefusesToFillMediaFromADifferentSender covers the
// store-side defence in depth for a connector that reports a reused
// remote id anyway: a connector's own id-reuse guard (see the whatsapp
// connector's reusesAnotherSendersID) is meant to stop this long before
// it reaches the store, but the store must never merge a message's
// media into an existing row just because it shares that row's
// (conversation, remote id) - it must also agree on who sent it, or
// Mallory could attach her own photo to Alice's already-stored message
// just by naming its id.
func TestAddMessage_RefusesToFillMediaFromADifferentSender(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{
		ID: "alice", ConversationID: "chat", RemoteID: "r1", SenderID: "alice", Text: "hi, it's Alice", Created: 1,
	})

	photo := &domain.Media{Kind: domain.MediaPhoto, FileName: "evil.jpg", Size: 10}
	again, inserted, err := s.AddMessage(ctx, domain.Message{
		ConversationID: "chat", RemoteID: "r1", SenderID: "mallory", Text: "hi, it's Alice", Created: 1, Media: photo,
	})
	if err != nil || inserted || again.ID != "alice" || again.Media != nil {
		t.Fatalf("AddMessage from a different sender reusing the id = %+v, inserted %t, %v; want Alice's message kept, with no media attached", again, inserted, err)
	}

	if got, _ := s.Message(ctx, "alice"); got.Media != nil {
		t.Errorf("media attached despite the sender mismatch: %+v", got.Media)
	}
}

// TestAddMessage_RefusesToFillMediaWhenOutgoingDiffers covers the same
// defence for a message this account sent itself: an incoming sender
// reusing its remote id must never be allowed to attach their own media
// to it either, even if some future caller ever let their sender id
// collide with how this account's own messages are stored.
func TestAddMessage_RefusesToFillMediaWhenOutgoingDiffers(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{
		ID: "own", ConversationID: "chat", RemoteID: "r1", Outgoing: true, Text: "sent from my phone", Created: 1,
	})

	photo := &domain.Media{Kind: domain.MediaPhoto, FileName: "evil.jpg", Size: 10}
	again, inserted, err := s.AddMessage(ctx, domain.Message{
		ConversationID: "chat", RemoteID: "r1", Outgoing: false, Text: "sent from my phone", Created: 1, Media: photo,
	})
	if err != nil || inserted || again.ID != "own" || again.Media != nil {
		t.Fatalf("AddMessage reusing our own message's id = %+v, inserted %t, %v; want it kept, with no media attached", again, inserted, err)
	}
}
