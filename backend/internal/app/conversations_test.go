package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestConversations_Searches(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "a", "Alex", domain.KindDirect)
	f.conversation(t, "b", "Bea", domain.KindDirect)

	got, err := f.commands.Conversations(t.Context(), "bea")
	if err != nil || len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("Conversations = %+v, %v", got, err)
	}
}

func TestOpenConversation_CreatesOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	if err := f.store.UpsertContact(ctx, domain.Contact{AccountID: "wa", RemoteID: "c1", Name: "New Contact"}); err != nil {
		t.Fatal(err)
	}

	first, err := f.commands.OpenConversation(ctx, "wa", "c1")
	if err != nil || first.Title != "New Contact" || first.Kind != domain.KindDirect {
		t.Fatalf("OpenConversation = %+v, %v", first, err)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	again, err := f.commands.OpenConversation(ctx, "wa", "c1")
	if err != nil || again.ID != first.ID || len(f.published.take()) != 0 {
		t.Fatalf("second OpenConversation = %+v, %v", again, err)
	}
}

func TestOpenConversation_Errors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	if _, err := f.commands.OpenConversation(ctx, "", ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("OpenConversation(empty) = %v, want ErrInvalidInput", err)
	}

	if _, err := f.commands.OpenConversation(ctx, "wa", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OpenConversation(unknown contact) = %v, want ErrNotFound", err)
	}
}

func TestMarkRead_ClearsUnreadAndTellsTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 1)
	f.published.take()

	if err := f.commands.MarkRead(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}

	want := []string{app.EventConversationUpdated, app.EventUnreadChanged}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	if !slices.Equal(f.dispatcher.read, []string{chat.ID}) {
		t.Errorf("read receipts = %v", f.dispatcher.read)
	}

	// Already read: nothing to publish, but the service still hears.
	if err := f.commands.MarkRead(ctx, chat.ID); err != nil || len(f.published.take()) != 0 {
		t.Errorf("second MarkRead = %v", err)
	}

	if err := f.commands.MarkRead(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkRead(missing) = %v, want ErrNotFound", err)
	}
}

// TestMarkRead_TellsTheServiceHowManyWereUnread confirms the
// conversation handed to the dispatcher's MarkRead still carries the
// unread count it had before this call cleared it locally: a service
// such as WhatsApp's connector needs that count to know how many of a
// conversation's newest messages to send a read receipt for, and
// fetching the conversation only after the local clear would always
// hand it zero, making every such call a silent no-op.
func TestMarkRead_TellsTheServiceHowManyWereUnread(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 3)
	f.published.take()

	if err := f.commands.MarkRead(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}

	if len(f.dispatcher.readConv) != 1 || f.dispatcher.readConv[0].Unread != 3 {
		t.Fatalf("dispatcher saw %+v, want one call with Unread 3", f.dispatcher.readConv)
	}
}

// TestMarkRead_SkipsTheServiceWithReadReceiptsOff checks incognito read
// receipts: the local unread count still clears, but the service is never
// told, so the phone and any other device keep showing the chat unread.
func TestMarkRead_SkipsTheServiceWithReadReceiptsOff(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 1)
	f.published.take()

	f.commands.ApplySettings(app.Settings{ReadReceipts: false})

	if err := f.commands.MarkRead(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}

	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || got.Unread != 0 {
		t.Errorf("unread = %d, %v, want 0 even with read receipts off", got.Unread, err)
	}

	if len(f.dispatcher.read) != 0 {
		t.Errorf("read receipts = %v, want none reaching the service", f.dispatcher.read)
	}
}

func TestSetMuted_UpdatesUnreadTotal(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 1)
	f.published.take()

	got, err := f.commands.SetMuted(ctx, chat.ID, true)
	if err != nil || !got.Muted {
		t.Fatalf("SetMuted = %+v, %v", got, err)
	}

	want := []string{app.EventConversationUpdated, app.EventUnreadChanged}
	if events := f.published.take(); !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}

	if f.commands.UnreadTotal(ctx) != 0 {
		t.Errorf("UnreadTotal = %d, want 0 while muted", f.commands.UnreadTotal(ctx))
	}

	if _, err := f.commands.SetMuted(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetMuted(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetPinned_StoresThenTellsTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.SetPinned(ctx, chat.ID, true)
	if err != nil || !got.Pinned {
		t.Fatalf("SetPinned = %+v, %v", got, err)
	}

	if !slices.Equal(f.organizer.pinned, []string{"chat true"}) {
		t.Errorf("organizer saw %v, want [chat true]", f.organizer.pinned)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	if _, err := f.commands.SetPinned(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetPinned(missing) = %v, want ErrNotFound", err)
	}
}

// TestSetPinned_ReportsAServiceFailureAsSafeInvalidInput mirrors
// archiveError's own fix (see
// TestSetArchived_ReportsAServiceFailureAsSafeInvalidInput): any
// failure from the organizer here, once the local pin already stands,
// must become a safe message naming what happened, logged with its
// safe class, instead of the generic internal error a request that
// changed nothing at all gets.
func TestSetPinned_ReportsAServiceFailureAsSafeInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.organizer.err = errors.New("offline")

	_, err := f.commands.SetPinned(ctx, chat.ID, true)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetPinned with a failing service = %v, want ErrInvalidInput", err)
	}
	if err != nil && !strings.Contains(err.Error(), "pinned here") {
		t.Errorf("SetPinned error = %q, want it to say the change was made here", err.Error())
	}

	// The local pin stands even though the service call failed.
	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || !got.Pinned {
		t.Errorf("Conversation after a failed SetPinned = %+v, %v; want Pinned true", got, err)
	}

	if lines := f.logger.take(); len(lines) != 1 || !strings.Contains(lines[0], "set pinned failed") {
		t.Errorf("logged = %v, want one line naming the failure", lines)
	}

	// Unpinning a failure wording says "unpinned", not "pinned".
	f.organizer.err = errors.New("offline again")
	if _, err := f.commands.SetPinned(ctx, chat.ID, false); err == nil || !strings.Contains(err.Error(), "unpinned here") {
		t.Errorf("SetPinned(false) with a failing service = %v, want it to say unpinned", err)
	}
}

func TestSetPinned_ReportsAPinLimitAsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.organizer.err = connector.ErrPinLimit

	if _, err := f.commands.SetPinned(ctx, chat.ID, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetPinned past the service's pin limit = %v, want ErrInvalidInput", err)
	}
}

func TestMovePin_SwapsAndPublishesBothConversations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	first := f.conversation(t, "first", "First", domain.KindDirect)
	second := f.conversation(t, "second", "Second", domain.KindDirect)

	if _, err := f.commands.SetPinned(ctx, first.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.SetPinned(ctx, second.ID, true); err != nil {
		t.Fatal(err)
	}
	f.published.take() // ignore the two pins' own events
	pinsBeforeMove := len(f.organizer.pinned)

	got, err := f.commands.MovePin(ctx, second.ID, "up")
	if err != nil || got.ID != second.ID {
		t.Fatalf("MovePin(second, up) = %+v, %v", got, err)
	}

	// Moving never asks a service anything, unlike SetPinned: it is
	// local reordering only.
	if len(f.organizer.pinned) != pinsBeforeMove {
		t.Errorf("organizer saw pins %v, want none added by MovePin", f.organizer.pinned)
	}

	published := f.published.take()
	if len(published) != 2 {
		t.Fatalf("events published = %v, want one per conversation swapped", published)
	}

	movedFirst, err := f.store.Conversation(ctx, first.ID)
	if err != nil || movedFirst.PinOrder <= 0 {
		t.Fatalf("Conversation(first) after MovePin = %+v, %v", movedFirst, err)
	}
	movedSecond, err := f.store.Conversation(ctx, second.ID)
	if err != nil || movedSecond.PinOrder >= movedFirst.PinOrder {
		t.Errorf("Conversation(second) after MovePin = %+v, want PinOrder before first (%d)", movedSecond, movedFirst.PinOrder)
	}
}

func TestMovePin_NoopAtTheEdgePublishesOnlyItself(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.published.take()

	got, err := f.commands.MovePin(ctx, chat.ID, "up")
	if err != nil || got.ID != chat.ID {
		t.Fatalf("MovePin(chat, up) at the top = %+v, %v", got, err)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v, want exactly one: a no-op move has no neighbor to publish", got)
	}
}

func TestMovePin_RejectsAnUnknownDirection(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.MovePin(ctx, chat.ID, "sideways"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("MovePin(sideways) = %v, want ErrInvalidInput", err)
	}
}

func TestMovePin_ReportsNotFoundForAnUnpinnedConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if _, err := f.commands.MovePin(ctx, chat.ID, "up"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MovePin(not pinned) = %v, want ErrNotFound", err)
	}
}

func TestSetHidden_StoresAndPublishesWithNoServiceCall(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.SetHidden(ctx, chat.ID, true)
	if err != nil || !got.Hidden {
		t.Fatalf("SetHidden = %+v, %v", got, err)
	}

	// Hiding is local to OmaMessenger only: it must never reach the
	// service, unlike pin and archive.
	if len(f.organizer.pinned) != 0 || len(f.organizer.archived) != 0 {
		t.Errorf("organizer saw pinned %v, archived %v, want neither called", f.organizer.pinned, f.organizer.archived)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	if _, err := f.commands.SetHidden(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetHidden(missing) = %v, want ErrNotFound", err)
	}
}

// TestMarkUnread_FlagsAnAlreadyReadConversationWithNoServiceCall checks
// the command palette's "Mark as unread" command: like SetHidden and
// SetReminder, this is local to OmaMessenger only, since there is
// nothing meaningful to tell a service that only ever reports how many
// messages it considers unread.
func TestMarkUnread_FlagsAnAlreadyReadConversationWithNoServiceCall(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.MarkUnread(ctx, chat.ID)
	if err != nil || got.Unread != 1 {
		t.Fatalf("MarkUnread = %+v, %v, want Unread 1", got, err)
	}

	if len(f.dispatcher.read) != 0 {
		t.Errorf("read receipts = %v, want none: marking unread is local only", f.dispatcher.read)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated, app.EventUnreadChanged}) {
		t.Errorf("events = %v", got)
	}

	if _, err := f.commands.MarkUnread(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkUnread(missing) = %v, want ErrNotFound", err)
	}
}

// TestMarkUnread_LeavesAnAlreadyUnreadConversationAlone checks that
// marking an already-unread conversation unread again never lowers
// whatever real count the service already reported, and does not
// publish a change nobody asked for.
func TestMarkUnread_LeavesAnAlreadyUnreadConversationAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.Unread(ctx, "wa", chat.RemoteID, 5)
	f.published.take()

	got, err := f.commands.MarkUnread(ctx, chat.ID)
	if err != nil || got.Unread != 5 {
		t.Fatalf("MarkUnread on an already-unread conversation = %+v, %v, want Unread unchanged at 5", got, err)
	}

	if len(f.published.take()) != 0 {
		t.Error("MarkUnread published a change for a conversation that was already unread")
	}
}

func TestSetReminder_StoresAndPublishesWithNoServiceCall(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.SetReminder(ctx, chat.ID, 5000)
	if err != nil || got.ReminderAt != 5000 {
		t.Fatalf("SetReminder = %+v, %v", got, err)
	}

	// Snoozing is local to OmaMessenger only: it must never reach the
	// service, unlike pin and archive.
	if len(f.organizer.pinned) != 0 || len(f.organizer.archived) != 0 {
		t.Errorf("organizer saw pinned %v, archived %v, want neither called", f.organizer.pinned, f.organizer.archived)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	cleared, err := f.commands.SetReminder(ctx, chat.ID, 0)
	if err != nil || cleared.ReminderAt != 0 {
		t.Fatalf("SetReminder(clear) = %+v, %v", cleared, err)
	}

	if _, err := f.commands.SetReminder(ctx, "missing", 5000); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetReminder(missing) = %v, want ErrNotFound", err)
	}
}
