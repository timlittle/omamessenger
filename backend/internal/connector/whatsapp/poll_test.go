package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// TestSavePoll_SavesACreationMessageForLaterVotes confirms a poll
// creation message's question and options are saved where vote.go's
// later tally lookups (pollstore.go) expect to find them.
func TestSavePoll_SavesACreationMessageForLaterVotes(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	msg := &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{
		Name: strPtr("Lunch?"),
		Options: []*waE2E.PollCreationMessage_Option{
			{OptionName: strPtr("Pizza")}, {OptionName: strPtr("Salad")},
		},
	}}

	savePoll(t.Context(), media, "chat", "m1", msg)

	shell, found, err := media.pollShell(t.Context(), "chat", "m1")
	if err != nil || !found {
		t.Fatalf("pollShell = %+v, found %t, %v", shell, found, err)
	}
	if shell.Question != "Lunch?" || len(shell.Options) != 2 {
		t.Errorf("saved poll = %+v, want the question and both options saved", shell)
	}
}

// TestSavePoll_DoesNothingWithoutAMediaStoreOrAPollMessage confirms
// savePoll is a safe no-op both for a connector built without a media
// store (see mediaFor's own doc comment) and for a message that is not
// a poll creation at all, rather than saving a hollow row for it.
func TestSavePoll_DoesNothingWithoutAMediaStoreOrAPollMessage(t *testing.T) {
	t.Parallel()

	plain := &waE2E.Message{Conversation: strPtr("hi")}

	savePoll(t.Context(), nil, "chat", "m1", plain) // must not panic with no media store

	media := newTestMediaStore(t)
	savePoll(t.Context(), media, "chat", "m1", plain)

	if _, found, err := media.pollShell(t.Context(), "chat", "m1"); err != nil || found {
		t.Errorf("pollShell after a non-poll message = found %t, %v, want not found", found, err)
	}
}
