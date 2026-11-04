// This test lives in its own file, not messages_test.go, only to stay
// under this codebase's file-length limit; it still mirrors
// messages.go's bumpConversation, the function it exercises.
package store_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAddMessage_UpdatesPreviewSenderVerified confirms bumpConversation
// carries a message's own SenderVerified into the conversation's
// preview_sender_verified, the same way it already does for the
// preview sender's name: a security review found a group's preview
// showing its newest sender unmarked even when that sender's own name
// was unverified, since the column to mark it with did not exist at
// all (see docs/decisions.md).
func TestAddMessage_UpdatesPreviewSenderVerified(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if _, _, err := s.AddMessage(ctx, domain.Message{
		ID: "stranger", ConversationID: "chat", RemoteID: "r1", Text: "send me the code",
		SenderName: "Mum", SenderVerified: false, Created: 100,
	}); err != nil {
		t.Fatal(err)
	}

	unverified, err := s.Conversation(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if unverified.PreviewSenderVerified {
		t.Error("PreviewSenderVerified = true, want false for an unverified sender")
	}

	if _, _, err := s.AddMessage(ctx, domain.Message{
		ID: "real", ConversationID: "chat", RemoteID: "r2", Text: "on my way",
		SenderName: "Real Mum", SenderVerified: true, Created: 200,
	}); err != nil {
		t.Fatal(err)
	}

	verified, err := s.Conversation(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if !verified.PreviewSenderVerified {
		t.Error("PreviewSenderVerified = false, want true for a verified sender's newer message")
	}
}
