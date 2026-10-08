package app_test

import (
	"errors"
	"os"
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

// TestDeleteMessages_DeletesAFailedSendLocallyAndRemovesItsCopy confirms
// a failed outgoing message - which never reached the service, so has
// no remote id - can still be deleted, with no call to the connector at
// all, and that doing so drops its outgoing attachment copy: it will
// never be retried once its message is gone.
func TestDeleteMessages_DeletesAFailedSendLocallyAndRemovesItsCopy(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)
	f.dispatcher.err = errors.New("offline")

	failed, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	copyPath := f.outgoing.Path(failed.ID, failed.Media.FileName)
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatalf("outgoing copy before delete: %v", err)
	}

	if err := f.commands.DeleteMessages(ctx, chat.ID, []string{failed.ID}, false); err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}

	if len(f.deleter.deleted) != 0 {
		t.Errorf("deleter was called = %v, want it never told about a message the service never saw", f.deleter.deleted)
	}

	if _, err := f.store.Message(ctx, failed.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Message after delete = %v, want ErrNotFound", err)
	}

	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Errorf("outgoing copy after delete = %v, want it removed", err)
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
