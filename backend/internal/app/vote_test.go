package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollMessage adds a message with an open poll to chat, for Vote's
// tests to cast a vote against.
func pollMessage(t *testing.T, f *fixture, chat domain.Conversation) domain.Message {
	t.Helper()

	m, _, err := f.store.AddMessage(t.Context(), domain.Message{
		ConversationID: chat.ID, RemoteID: "40", Text: "[Poll: Lunch?]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{
			Question: "Lunch?",
			Options:  []domain.PollOption{{ID: "a", Text: "Pizza"}, {ID: "b", Text: "Salad"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestVote_TellsTheServiceAndReturnsTheUpdatedMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := pollMessage(t, f, chat)

	got, err := f.commands.Vote(t.Context(), m.ID, []string{"a"})
	if err != nil || got.ID != m.ID {
		t.Fatalf("Vote = %+v, %v", got, err)
	}

	if !slices.Equal(f.voter.voted, []string{"40 [a]"}) {
		t.Errorf("voter saw %v, want [40 [a]]", f.voter.voted)
	}
}

func TestVote_RejectsAnEmptyMessageID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.Vote(t.Context(), "", []string{"a"}); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote(empty) = %v, want ErrInvalidInput", err)
	}
}

func TestVote_RejectsNoOptions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := pollMessage(t, f, chat)

	if _, err := f.commands.Vote(t.Context(), m.ID, nil); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote(no options) = %v, want ErrInvalidInput", err)
	}
}

func TestVote_RejectsAMessageWithNoPoll(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(t.Context(), domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.Vote(t.Context(), m.ID, []string{"a"}); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote(no poll) = %v, want ErrInvalidInput", err)
	}
}

func TestVote_RejectsAClosedPoll(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(t.Context(), domain.Message{
		ConversationID: chat.ID, RemoteID: "40", Text: "[Poll: Lunch?]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{Question: "Lunch?", Closed: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.Vote(t.Context(), m.ID, []string{"a"}); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote(closed poll) = %v, want ErrInvalidInput", err)
	}

	if len(f.voter.voted) != 0 {
		t.Errorf("voter was asked for a closed poll: %v", f.voter.voted)
	}
}

func TestVote_RejectsAMessageNeverSentToTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(t.Context(), domain.Message{
		ConversationID: chat.ID, Text: "hi", Created: 1, Outgoing: true, Status: domain.StatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.Vote(t.Context(), m.ID, []string{"a"}); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote(no remote id) = %v, want ErrInvalidInput", err)
	}
}

func TestVote_MapsAnUnsupportedConnectorToInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := pollMessage(t, f, chat)

	f.voter.err = connector.ErrNoVoter
	if _, err := f.commands.Vote(t.Context(), m.ID, []string{"a"}); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Vote with an unsupported connector = %v, want ErrInvalidInput", err)
	}
}

func TestVote_ReturnsAnOtherwiseUnexpectedFailureUnchanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := pollMessage(t, f, chat)

	f.voter.err = errors.New("offline")
	if _, err := f.commands.Vote(t.Context(), m.ID, []string{"a"}); !errors.Is(err, f.voter.err) {
		t.Errorf("Vote with a failing service = %v, want %v", err, f.voter.err)
	}
}
