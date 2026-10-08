package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetPoll_StoresAFreshPollOnAMessageWithNoMedia(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "[Poll: Lunch?]", Created: 1})

	poll := domain.Poll{Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}}}
	got, found, err := s.SetPoll(ctx, "chat", "r1", poll)
	if err != nil || !found || got.Media == nil || got.Media.Poll == nil {
		t.Fatalf("SetPoll = %+v, found %t, %v", got, found, err)
	}
	if got.Media.Kind != domain.MediaPoll || got.Media.Poll.Question != "Lunch?" {
		t.Errorf("stored media = %+v, want a poll asking Lunch?", got.Media)
	}

	stored, err := s.Message(ctx, "m1")
	if err != nil || stored.Media == nil || stored.Media.Poll == nil || stored.Media.Poll.Question != "Lunch?" {
		t.Errorf("reloaded message = %+v, %v", stored, err)
	}
}

func TestSetPoll_MergesATalliesOnlyUpdateOntoTheStoredPoll(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	first := &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{
		Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}, {ID: "b", Text: "Salad"}},
	}}
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "r1", Text: "[Poll: Lunch?]", Media: first, Created: 1})

	update := domain.Poll{Options: []domain.PollOption{{ID: "a", Votes: 2, Chosen: true}, {ID: "b", Votes: 1}}, TotalVoters: 3}
	got, found, err := s.SetPoll(ctx, "chat", "r1", update)
	if err != nil || !found {
		t.Fatalf("SetPoll = %+v, found %t, %v", got, found, err)
	}

	if got.Media.Poll.Question != "Lunch?" {
		t.Errorf("Question = %q, want kept from the first report", got.Media.Poll.Question)
	}
	want := []domain.PollOption{{ID: "a", Text: "Pizza", Votes: 2, Chosen: true}, {ID: "b", Text: "Salad", Votes: 1}}
	if !slices.Equal(got.Media.Poll.Options, want) {
		t.Errorf("Options = %+v, want %+v", got.Media.Poll.Options, want)
	}
	if got.Media.Poll.TotalVoters != 3 {
		t.Errorf("TotalVoters = %d, want 3", got.Media.Poll.TotalVoters)
	}
}

func TestSetPoll_IgnoresAMessageThatIsNotStored(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	got, found, err := s.SetPoll(ctx, "chat", "missing", domain.Poll{Question: "Lunch?"})
	if err != nil || found || got.ID != "" {
		t.Fatalf("SetPoll(missing) = %+v, found %t, %v; want ignored", got, found, err)
	}
}
