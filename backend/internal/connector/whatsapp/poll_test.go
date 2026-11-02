package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
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

// TestHandleMessage_KeepsTheFirstSendersPollWhenAnotherOneReusesTheID
// covers the same id-reuse fix as keys.go's putMessageKey and
// storage.go's media_refs, for poll_meta and poll_options: Mallory
// naming Alice's already-saved poll id in her own poll creation
// message must never let her question and options replace Alice's.
func TestHandleMessage_KeepsTheFirstSendersPollWhenAnotherOneReusesTheID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	alice := types.NewJID("15551111111", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)

	alicePoll := &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{
		Name:    strPtr("Lunch?"),
		Options: []*waE2E.PollCreationMessage_Option{{OptionName: strPtr("Pizza")}},
	}}
	malloryPoll := &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{
		Name:    strPtr("Send gift cards to this number"),
		Options: []*waE2E.PollCreationMessage_Option{{OptionName: strPtr("Yes")}},
	}}

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: alice}, ID: "M0", Timestamp: time.Unix(1, 0)},
		Message: alicePoll,
	})
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: malloryPoll,
	})

	shell, found, err := media.pollShell(t.Context(), remoteID(chat), "M0")
	if err != nil || !found || shell.Question != "Lunch?" {
		t.Errorf("saved poll = %+v, found %t, %v, want Alice's own question kept, not overwritten by the id reused by Mallory", shell, found, err)
	}
}
