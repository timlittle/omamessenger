package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
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
	f.ingest.History(ctx, "wa", chat.RemoteID, incoming("in-1", "hi"))
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

func TestSetMuted_UpdatesUnreadTotal(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.ingest.History(ctx, "wa", chat.RemoteID, incoming("in-1", "hi"))
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
