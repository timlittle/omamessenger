package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestReact_TellsTheServiceAndReturnsTheUpdatedMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := sentMessage(t, f, chat)

	got, err := f.commands.React(ctx, m.ID, "👍")
	if err != nil || got.ID != m.ID {
		t.Fatalf("React = %+v, %v", got, err)
	}

	if !slices.Equal(f.reactor.reacted, []string{"40 👍"}) {
		t.Errorf("reactor saw %v, want [40 👍]", f.reactor.reacted)
	}
}

func TestReact_RejectsAnEmptyMessageID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.React(t.Context(), "", "👍"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("React(empty) = %v, want ErrInvalidInput", err)
	}
}

func TestReact_RejectsAMessageNeverSentToTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{
		ConversationID: chat.ID, Text: "hi", Created: 1, Outgoing: true, Status: domain.StatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.React(ctx, m.ID, "👍"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("React(no remote id) = %v, want ErrInvalidInput", err)
	}
}

// TestReact_MapsConnectorFailures confirms React maps an unsupported
// connector to ErrInvalidInput but passes an otherwise unexpected
// failure through unchanged, the same as DeleteMessages and Vote.
func TestReact_MapsConnectorFailures(t *testing.T) {
	t.Parallel()

	offline := errors.New("offline")
	cases := []struct {
		name       string
		reactorErr error
		want       error
	}{
		{"an unsupported connector maps to invalid input", connector.ErrNoReactions, app.ErrInvalidInput},
		{"an otherwise unexpected failure is unchanged", offline, offline},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
			m := sentMessage(t, f, chat)

			f.reactor.err = c.reactorErr
			if _, err := f.commands.React(ctx, m.ID, "👍"); !errors.Is(err, c.want) {
				t.Errorf("React = %v, want %v", err, c.want)
			}
		})
	}
}
