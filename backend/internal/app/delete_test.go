package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestDeleteMessages_TellsTheServiceAndRemovesTheMessageLocally(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{m.ID}, true); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(f.deleter.deleted, []string{chat.ID + " 40 true"}) {
		t.Errorf("deleter saw %v, want [%q]", f.deleter.deleted, chat.ID+" 40 true")
	}

	if _, err := f.store.Message(ctx, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Message after delete = %v, want ErrNotFound", err)
	}

	if published := f.published.take(); !slices.Contains(published, app.EventMessageRemoved) {
		t.Errorf("published = %v, want a %s event", published, app.EventMessageRemoved)
	}
}

func TestDeleteMessages_RejectsAnEmptyConversationID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.DeleteMessages(t.Context(), "", []string{"m1"}, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("DeleteMessages(empty conversation) = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteMessages_RejectsNoMessageIDs(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if err := f.commands.DeleteMessages(ctx, chat.ID, nil, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("DeleteMessages(no ids) = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteMessages_RejectsAMessageNeverSentToTheService(t *testing.T) {
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

	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{m.ID}, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("DeleteMessages(no remote id) = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteMessages_RejectsAMessageFromAnotherConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	other := f.conversation(t, "other", "Other", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: other.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{m.ID}, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("DeleteMessages(wrong conversation) = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteMessages_MapsAnUnsupportedConnectorToInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	f.deleter.err = connector.ErrNoDeleter
	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{m.ID}, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("DeleteMessages with an unsupported connector = %v, want ErrInvalidInput", err)
	}

	if _, err := f.store.Message(ctx, m.ID); err != nil {
		t.Errorf("Message after a refused delete = %v, want it still stored", err)
	}
}

func TestDeleteMessages_ReturnsAnOtherwiseUnexpectedFailureUnchanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{ConversationID: chat.ID, RemoteID: "40", Text: "hi", Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	f.deleter.err = errors.New("offline")
	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{m.ID}, true); !errors.Is(err, f.deleter.err) {
		t.Errorf("DeleteMessages with a failing service = %v, want %v", err, f.deleter.err)
	}
}
