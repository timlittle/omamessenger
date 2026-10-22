package app_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestPollUpdated_UpdatesTheStoredPollAndPublishes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	m := incoming("e1", "[Poll: Lunch?]")
	m.Media = &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{
		Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}},
	}}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, m)
	f.published.take()

	f.ingest.PollUpdated(ctx, "wa", chat.RemoteID, "e1", domain.Poll{
		Options: []domain.PollOption{{ID: "a", Votes: 2, Chosen: true}}, TotalVoters: 2,
	})

	if got := f.published.take(); !slices.Equal(got, []string{app.EventMessageUpdated}) {
		t.Errorf("events = %v, want %v", got, []string{app.EventMessageUpdated})
	}

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Media == nil || messages[0].Media.Poll == nil {
		t.Fatalf("messages = %+v, %v", messages, err)
	}
	poll := messages[0].Media.Poll
	if poll.Question != "Lunch?" || poll.TotalVoters != 2 || poll.Options[0].Votes != 2 || !poll.Options[0].Chosen {
		t.Errorf("poll = %+v, want the question kept and the tally updated", poll)
	}
}

// TestPollUpdated_CapsOptionCountAndText reproduces a live poll update
// reporting more options, or longer option text, than domain's own
// caps allow: a vote or tally reported this way, without a full
// message, must never bypass the same caps domain.SanitizeIncoming
// gives a poll arriving as part of one.
func TestPollUpdated_CapsOptionCountAndText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	m := incoming("e1", "[Poll: Lunch?]")
	m.Media = &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{
		Question: "Lunch?", Options: []domain.PollOption{{ID: "a", Text: "Pizza"}},
	}}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, m)
	f.published.take()

	options := make([]domain.PollOption, domain.MaxPollOptions+20)
	for i := range options {
		options[i] = domain.PollOption{ID: "a", Text: strings.Repeat("x", domain.MaxPollOptionTextLength+10)}
	}
	f.ingest.PollUpdated(ctx, "wa", chat.RemoteID, "e1", domain.Poll{
		Question: strings.Repeat("q", domain.MaxPollQuestionLength+10), Options: options,
	})

	messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Media == nil || messages[0].Media.Poll == nil {
		t.Fatalf("messages = %+v, %v", messages, err)
	}
	poll := messages[0].Media.Poll
	if got := len(poll.Options); got != domain.MaxPollOptions {
		t.Errorf("stored %d poll options, want exactly %d", got, domain.MaxPollOptions)
	}
	if n := domain.UTF16Len(poll.Question); n > domain.MaxPollQuestionLength {
		t.Errorf("poll question is %d UTF-16 units, want at most %d", n, domain.MaxPollQuestionLength)
	}
	for _, o := range poll.Options {
		if n := domain.UTF16Len(o.Text); n > domain.MaxPollOptionTextLength {
			t.Errorf("poll option text is %d UTF-16 units, want at most %d", n, domain.MaxPollOptionTextLength)
		}
	}
}

func TestPollUpdated_IgnoresUnknownConversationsAndMessages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	f.ingest.PollUpdated(ctx, "wa", "nowhere", "e1", domain.Poll{Question: "Lunch?"})
	f.ingest.PollUpdated(ctx, "wa", chat.RemoteID, "missing", domain.Poll{Question: "Lunch?"})

	if len(f.published.take()) != 0 {
		t.Error("an unknown poll update published an event")
	}
}
