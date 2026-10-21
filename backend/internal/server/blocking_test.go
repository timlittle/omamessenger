package server_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestContactsBlock_BlocksAndUnblocks exercises contacts.block and
// contacts.unblock together against the seeded direct conversation
// "chat".
func TestContactsBlock_BlocksAndUnblocks(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	blocked, err := call[domain.Conversation](t, s, "contacts.block", map[string]string{"conversationId": "chat"})
	if err != nil || !blocked.Blocked {
		t.Fatalf("contacts.block = %+v, %v", blocked, err)
	}

	unblocked, err := call[domain.Conversation](t, s, "contacts.unblock", map[string]string{"conversationId": "chat"})
	if err != nil || unblocked.Blocked {
		t.Fatalf("contacts.unblock = %+v, %v", unblocked, err)
	}
}

// TestConversationsReportSpam_BlocksAndHides checks that reporting the
// seeded direct conversation as spam blocks its contact and hides it
// from the standard list.
func TestConversationsReportSpam_BlocksAndHides(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	reported, err := call[domain.Conversation](t, s, "conversations.reportSpam", map[string]string{"conversationId": "chat"})
	if err != nil || !reported.Blocked || !reported.Hidden {
		t.Fatalf("conversations.reportSpam = %+v, %v, want Blocked and Hidden true", reported, err)
	}
}

// TestContactsBlock_RejectsAnUnknownConversation reports invalid params
// for a conversation id that is not actually stored.
func TestContactsBlock_RejectsAnUnknownConversation(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[domain.Conversation](t, s, "contacts.block", map[string]string{"conversationId": "missing"}); code(err) == 0 {
		t.Error("contacts.block(missing) succeeded, want an error")
	}
}
