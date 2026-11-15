package domain_test

// limits_conversation_test.go covers domain.ValidConversation, kept
// apart from limits_test.go the same way limits_messageid_test.go
// already is: this checks a conversation can be stored at all, not a
// field capped in place.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestValidConversation_AcceptsAccountRemoteIDAndTitle confirms a
// conversation carrying all three required fields passes.
func TestValidConversation_AcceptsAccountRemoteIDAndTitle(t *testing.T) {
	t.Parallel()

	c := domain.Conversation{AccountID: "acc1", RemoteID: "r1", Title: "Alex"}
	if !domain.ValidConversation(c) {
		t.Errorf("ValidConversation(%+v) = false, want true", c)
	}
}

// TestValidConversation_RejectsMissingField confirms a conversation
// missing any one of AccountID, RemoteID or Title is rejected: the
// store cannot key a conversation, and every later message for it,
// without all three.
func TestValidConversation_RejectsMissingField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		c    domain.Conversation
	}{
		{"missing account", domain.Conversation{RemoteID: "r1", Title: "Alex"}},
		{"missing remote id", domain.Conversation{AccountID: "acc1", Title: "Alex"}},
		{"missing title", domain.Conversation{AccountID: "acc1", RemoteID: "r1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if domain.ValidConversation(tt.c) {
				t.Errorf("ValidConversation(%+v) = true, want false", tt.c)
			}
		})
	}
}

// TestHasText checks the rule the store applies to every message: a
// message without text is refused, so a connector must give one.
func TestHasText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    domain.Message
		want bool
	}{
		{"text", domain.Message{Text: "hi"}, true},
		{"placeholder", domain.Message{Text: domain.MediaPlaceholder(domain.MediaPhoto)}, true},
		{"empty", domain.Message{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := domain.HasText(tt.m); got != tt.want {
				t.Errorf("HasText() = %t, want %t", got, tt.want)
			}
		})
	}
}
