package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// sentMessage adds a message to chat that already reached the service,
// with remote id "40", for a test that reacts to, votes on or deletes a
// message the connector already knows about. It sits here, beside
// pollMessage, rather than in helpers_test.go only to keep that file
// under this repository's line limit.
func sentMessage(t *testing.T, f *fixture, chat domain.Conversation) domain.Message {
	t.Helper()

	m, _, err := f.store.AddMessage(t.Context(), domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	return m
}

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

// TestVote_RejectsInvalidInputAndServiceFailures confirms every input
// Vote refuses before reaching the connector, that a closed poll never
// even asks the voter, and that a connector failure is mapped or passed
// through the same way React and DeleteMessages map theirs.
func TestVote_RejectsInvalidInputAndServiceFailures(t *testing.T) {
	t.Parallel()

	offline := errors.New("offline")
	cases := []struct {
		name string
		// setup stores whatever the case needs and returns the message
		// id and options to vote with.
		setup func(t *testing.T, f *fixture) (messageID string, options []string)
		// voterErr, when set, makes the fake voter fail with it before
		// Vote is called.
		voterErr error
		want     error
		// checkVoterNotAsked confirms the case was rejected before any
		// call could reach the voter.
		checkVoterNotAsked bool
	}{
		{
			name:  "an empty message id",
			setup: func(t *testing.T, f *fixture) (string, []string) { return "", []string{"a"} },
			want:  app.ErrInvalidInput,
		},
		{
			name: "no options",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				return pollMessage(t, f, chat).ID, nil
			},
			want: app.ErrInvalidInput,
		},
		{
			name: "a message with no poll",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				return sentMessage(t, f, chat).ID, []string{"a"}
			},
			want: app.ErrInvalidInput,
		},
		{
			name: "a closed poll",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				m, _, err := f.store.AddMessage(t.Context(), domain.Message{
					ConversationID: chat.ID, RemoteID: "40", Text: "[Poll: Lunch?]", Created: 1,
					Media: &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{Question: "Lunch?", Closed: true}},
				})
				if err != nil {
					t.Fatal(err)
				}

				return m.ID, []string{"a"}
			},
			want:               app.ErrInvalidInput,
			checkVoterNotAsked: true,
		},
		{
			name: "a message never sent to the service",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				m, _, err := f.store.AddMessage(t.Context(), domain.Message{
					ConversationID: chat.ID, Text: "hi", Created: 1, Outgoing: true, Status: domain.StatusPending,
				})
				if err != nil {
					t.Fatal(err)
				}

				return m.ID, []string{"a"}
			},
			want: app.ErrInvalidInput,
		},
		{
			name: "an unsupported connector maps to invalid input",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				return pollMessage(t, f, chat).ID, []string{"a"}
			},
			voterErr: connector.ErrNoVoter,
			want:     app.ErrInvalidInput,
		},
		{
			name: "an otherwise unexpected failure is unchanged",
			setup: func(t *testing.T, f *fixture) (string, []string) {
				chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
				return pollMessage(t, f, chat).ID, []string{"a"}
			},
			voterErr: offline,
			want:     offline,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			messageID, options := c.setup(t, f)
			f.voter.err = c.voterErr

			if _, err := f.commands.Vote(t.Context(), messageID, options); !errors.Is(err, c.want) {
				t.Errorf("Vote = %v, want %v", err, c.want)
			}

			if c.checkVoterNotAsked && len(f.voter.voted) != 0 {
				t.Errorf("voter was asked for a closed poll: %v", f.voter.voted)
			}
		})
	}
}
