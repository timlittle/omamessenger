package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// outgoingSentMessage adds an outgoing text message already reported
// sent to the service, as an edit's tests need: unlike sentMessage, it
// is marked Outgoing so EditMessage accepts it.
func outgoingSentMessage(t *testing.T, f *fixture, chat domain.Conversation, text string) domain.Message {
	t.Helper()

	m, _, err := f.store.AddMessage(t.Context(), domain.Message{
		ConversationID: chat.ID, RemoteID: "40", Text: text, Outgoing: true, Status: domain.StatusSent, Created: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestEditMessage_UpdatesTheStoreAndTellsTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	got, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there")
	if err != nil || got.Text != "hi there" || !got.Edited {
		t.Fatalf("EditMessage = %+v, %v", got, err)
	}

	stored, err := f.store.Message(ctx, m.ID)
	if err != nil || stored.Text != "hi there" {
		t.Fatalf("stored message = %+v, %v", stored, err)
	}

	if len(f.editor.edited) != 1 || f.editor.edited[0] != "40 hi there" {
		t.Errorf("editor saw %v, want one edit of 40 to %q", f.editor.edited, "hi there")
	}
}

func TestEditMessage_RejectsEmptyText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "   "); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(blank) = %v, want ErrInvalidInput", err)
	}
}

func TestEditMessage_RejectsTextOverTheLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, strings.Repeat("a", domain.MaxTextLength+1)); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(too long) = %v, want ErrInvalidInput", err)
	}
}

func TestEditMessage_RejectsAMissingMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if _, err := f.commands.EditMessage(ctx, chat.ID, "missing", "hi there"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("EditMessage(missing message) = %v, want ErrNotFound", err)
	}
}

func TestEditMessage_RejectsAnIncomingMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{
		ConversationID: chat.ID, RemoteID: "41", Text: "hi", Status: domain.StatusReceived, Created: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(incoming) = %v, want ErrInvalidInput", err)
	}
}

func TestEditMessage_RejectsAMessageNeverSentToTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{
		ConversationID: chat.ID, Text: "hi", Outgoing: true, Status: domain.StatusPending, Created: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(no remote id) = %v, want ErrInvalidInput", err)
	}
}

func TestEditMessage_RejectsAMessageWithMedia(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m, _, err := f.store.AddMessage(ctx, domain.Message{
		ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Outgoing: true, Status: domain.StatusSent, Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "caption"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(media) = %v, want ErrInvalidInput", err)
	}
}

func TestEditMessage_RejectsAMessageFromAnotherConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	other := f.conversation(t, "other", "Other", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	if _, err := f.commands.EditMessage(ctx, other.ID, m.ID, "hi there"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("EditMessage(wrong conversation) = %v, want ErrInvalidInput", err)
	}
}

// TestEditMessage_RevertsOnConnectorFailure confirms a connector refusal
// leaves the stored message exactly as it was before the optimistic
// update, and maps the same way React and DeleteMessages do.
func TestEditMessage_RevertsOnConnectorFailure(t *testing.T) {
	t.Parallel()

	offline := errors.New("offline")
	cases := []struct {
		name    string
		editErr error
		wantErr error
	}{
		{"an unsupported connector maps to invalid input", connector.ErrEditUnsupported, app.ErrInvalidInput},
		{"an otherwise unexpected failure is unchanged", offline, offline},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
			m := outgoingSentMessage(t, f, chat, "hi")

			f.editor.err = c.editErr
			if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there"); !errors.Is(err, c.wantErr) {
				t.Errorf("EditMessage = %v, want %v", err, c.wantErr)
			}

			stored, err := f.store.Message(ctx, m.ID)
			if err != nil || stored.Text != "hi" {
				t.Errorf("stored message after a failed edit = %+v, %v, want the original text restored", stored, err)
			}
		})
	}
}

// TestEditMessage_RevertIsBestEffortWhenTheStoreCannotBeWrittenTo
// covers revertEdit's own best-effort guard: a connector refusal whose
// revert cannot even reach the store - here, because the database
// closed in the moment between the optimistic update and the
// connector's answer - still reports the connector's own refusal to
// the caller, exactly as TestEditMessage_RevertsOnConnectorFailure
// does, rather than failing on the revert itself or panicking.
func TestEditMessage_RevertIsBestEffortWhenTheStoreCannotBeWrittenTo(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	f.editor.err = errors.New("offline")
	f.editor.onEditMessage = func() { _ = f.store.Close() } // already closed again in t.Cleanup; Close tolerates that

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there"); !errors.Is(err, f.editor.err) {
		t.Errorf("EditMessage = %v, want %v", err, f.editor.err)
	}
}

// TestEditMessage_OwnEchoDoesNotDuplicate confirms that the service
// reporting our own edit back, through Ingest.Edited, lands as a no-op
// update rather than a second message: both paths key off the same
// remote id.
func TestEditMessage_OwnEchoDoesNotDuplicate(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := outgoingSentMessage(t, f, chat, "hi")

	if _, err := f.commands.EditMessage(ctx, chat.ID, m.ID, "hi there"); err != nil {
		t.Fatal(err)
	}

	f.ingest.Edited(ctx, "wa", chat.RemoteID, domain.Message{RemoteID: "40", Text: "hi there"})

	page, _, err := f.store.Messages(ctx, chat.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(page) != 1 || page[0].Text != "hi there" {
		t.Errorf("messages after the echo = %+v, want exactly one, unchanged by the echo", page)
	}
}
