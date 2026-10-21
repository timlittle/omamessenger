package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestBlockContact_TellsTheServiceThenStores(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.BlockContact(ctx, chat.ID, true)
	if err != nil || !got.Blocked {
		t.Fatalf("BlockContact = %+v, %v", got, err)
	}

	if !slices.Equal(f.blocker.blocked, []string{"chat true"}) {
		t.Errorf("blocker saw %v, want [chat true]", f.blocker.blocked)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	unblocked, err := f.commands.BlockContact(ctx, chat.ID, false)
	if err != nil || unblocked.Blocked {
		t.Fatalf("BlockContact(unblock) = %+v, %v", unblocked, err)
	}

	if _, err := f.commands.BlockContact(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("BlockContact(missing) = %v, want ErrNotFound", err)
	}
}

func TestBlockContact_RefusesAGroup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	group := f.conversation(t, "group", "Group", domain.KindGroup)

	if _, err := f.commands.BlockContact(ctx, group.ID, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("BlockContact(group) = %v, want ErrInvalidInput", err)
	}

	if len(f.blocker.blocked) != 0 {
		t.Errorf("blocker saw %v, want nothing asked for a group", f.blocker.blocked)
	}
}

// TestBlockContact_NeverStoresAFailedBlock checks that a connector
// refusal leaves the conversation's blocked flag untouched, rather
// than persisting a block that never actually happened with the
// service.
func TestBlockContact_NeverStoresAFailedBlock(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.blocker.err = errors.New("offline")

	if _, err := f.commands.BlockContact(ctx, chat.ID, true); !errors.Is(err, f.blocker.err) {
		t.Errorf("BlockContact with a failing service = %v, want %v", err, f.blocker.err)
	}

	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || got.Blocked {
		t.Errorf("Conversation after a failed BlockContact = %+v, %v; want Blocked false", got, err)
	}
}

func TestBlockContact_ReportsAnUnsupportedServiceAsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.blocker.err = connector.ErrNoBlocker

	if _, err := f.commands.BlockContact(ctx, chat.ID, true); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("BlockContact without a Blocker = %v, want ErrInvalidInput", err)
	}
}

func TestReportSpam_ReportsBlocksAndHides(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.ReportSpam(ctx, chat.ID)
	if err != nil || !got.Blocked || !got.Hidden {
		t.Fatalf("ReportSpam = %+v, %v, want Blocked and Hidden true", got, err)
	}

	if f.spamReport.reported != 1 {
		t.Errorf("spam reports = %d, want 1", f.spamReport.reported)
	}
	if !slices.Equal(f.blocker.blocked, []string{"chat true"}) {
		t.Errorf("blocker saw %v, want [chat true]", f.blocker.blocked)
	}

	// Archiving is deliberately not used: see block.go's own doc
	// comment on why hidden is the local-only flag here.
	if got.Archived {
		t.Errorf("ReportSpam archived the conversation, want it left alone")
	}

	if _, err := f.commands.ReportSpam(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReportSpam(missing) = %v, want ErrNotFound", err)
	}
}

func TestReportSpam_RefusesAGroup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	group := f.conversation(t, "group", "Group", domain.KindGroup)

	if _, err := f.commands.ReportSpam(ctx, group.ID); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("ReportSpam(group) = %v, want ErrInvalidInput", err)
	}

	if f.spamReport.reported != 0 || len(f.blocker.blocked) != 0 {
		t.Errorf("spam reports = %d, blocker saw %v, want neither called for a group", f.spamReport.reported, f.blocker.blocked)
	}
}

// TestReportSpam_StopsBeforeBlockingWhenTheReportFails checks that a
// failed report never blocks or hides the conversation on its own:
// the caller sees the failure and nothing else silently happened.
func TestReportSpam_StopsBeforeBlockingWhenTheReportFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.spamReport.err = errors.New("offline")

	if _, err := f.commands.ReportSpam(ctx, chat.ID); !errors.Is(err, f.spamReport.err) {
		t.Errorf("ReportSpam with a failing report = %v, want %v", err, f.spamReport.err)
	}

	if len(f.blocker.blocked) != 0 {
		t.Errorf("blocker saw %v, want nothing asked after a failed report", f.blocker.blocked)
	}

	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || got.Blocked || got.Hidden {
		t.Errorf("Conversation after a failed ReportSpam = %+v, %v; want neither Blocked nor Hidden", got, err)
	}
}
