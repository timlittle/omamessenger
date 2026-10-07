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
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

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

func TestReact_MapsAnUnsupportedConnectorToInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	f.reactor.err = connector.ErrNoReactions
	if _, err := f.commands.React(ctx, m.ID, "👍"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("React with an unsupported connector = %v, want ErrInvalidInput", err)
	}
}

func TestReact_ReturnsAnOtherwiseUnexpectedFailureUnchanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	f.reactor.err = errors.New("offline")
	if _, err := f.commands.React(ctx, m.ID, "👍"); !errors.Is(err, f.reactor.err) {
		t.Errorf("React with a failing service = %v, want %v", err, f.reactor.err)
	}
}
