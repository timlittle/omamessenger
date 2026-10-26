package app_test

// conversations_archive_test.go holds SetArchived's own tests, split out
// of conversations_test.go once that file passed revive's file-length
// gate: SetArchived gained several tests for restoring a pin archiving
// cleared (see conversations.go's unpinForArchive and
// restorePinForUnarchive), the same reasoning connector_fakes_test.go's
// own split from helpers_test.go already used.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetArchived_StoresThenTellsTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	got, err := f.commands.SetArchived(ctx, chat.ID, true)
	if err != nil || !got.Archived {
		t.Fatalf("SetArchived = %+v, %v", got, err)
	}

	if !slices.Equal(f.organizer.archived, []string{"chat true"}) {
		t.Errorf("organizer saw %v, want [chat true]", f.organizer.archived)
	}

	if got := f.published.take(); !slices.Equal(got, []string{app.EventConversationUpdated}) {
		t.Errorf("events = %v", got)
	}

	if _, err := f.commands.SetArchived(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetArchived(missing) = %v, want ErrNotFound", err)
	}
}

// TestSetArchived_UnpinsLocallyForAServiceThatRequiresIt reproduces a
// report where a pinned WhatsApp chat got archived and the list then
// showed it both pinned and archived at once, a combination the real
// service never has. WhatsApp's own archive always unpins too (see
// whatsapp/organize.go), so the local store must do the same in the
// same call, rather than only once the service's own round trip
// reports it back: that correction never arrives if the service call
// fails, which is exactly how the stale pin got stuck in the report.
func TestSetArchived_UnpinsLocallyForAServiceThatRequiresIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.published.take()
	f.archiveUnpinner.unpins["wa"] = true

	// The service call itself fails, the way a rejected app-state patch
	// did in the report: the local pin must still clear, since the
	// service never has a conversation both pinned and archived.
	f.organizer.err = errors.New("offline")

	got, err := f.commands.SetArchived(ctx, chat.ID, true)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("SetArchived = %v, want ErrInvalidInput", err)
	}
	if got.Pinned || !got.Archived {
		t.Errorf("SetArchived result = %+v, want pinned false and archived true", got)
	}

	if stored, err := f.store.Conversation(ctx, chat.ID); err != nil || stored.Pinned || !stored.Archived {
		t.Errorf("Conversation after SetArchived = %+v, %v; want pinned false and archived true", stored, err)
	}
}

// TestSetArchived_RestoresThePinOnUnarchive reproduces the follow-up to
// the report above: a chat archived while pinned, for a service that
// unpins on archive, comes back pinned once it is unarchived, both
// locally and with the service, rather than staying unpinned for good
// just because archiving once took the pin away.
func TestSetArchived_RestoresThePinOnUnarchive(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.published.take()
	f.archiveUnpinner.unpins["wa"] = true

	if _, err := f.commands.SetArchived(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	got, err := f.commands.SetArchived(ctx, chat.ID, false)
	if err != nil {
		t.Fatalf("SetArchived(false) = %v, %v", got, err)
	}
	if !got.Pinned || got.Archived {
		t.Errorf("SetArchived(false) result = %+v, want pinned true and archived false", got)
	}

	if stored, err := f.store.Conversation(ctx, chat.ID); err != nil || !stored.Pinned {
		t.Errorf("Conversation after unarchiving = %+v, %v; want Pinned true", stored, err)
	}

	// The service hears the re-pin too, not only the local store.
	if !slices.Equal(f.organizer.pinned, []string{"chat true", "chat true"}) {
		t.Errorf("organizer saw pins %v, want the original pin and the re-pin after unarchiving", f.organizer.pinned)
	}
}

// TestSetArchived_NeverPinsOnUnarchiveForAChatThatWasNeverPinned checks
// the ordinary case stays ordinary: a chat archived while unpinned
// stays unpinned once it comes back, with no service call about
// pinning at all.
func TestSetArchived_NeverPinsOnUnarchiveForAChatThatWasNeverPinned(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.archiveUnpinner.unpins["wa"] = true

	if _, err := f.commands.SetArchived(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := f.commands.SetArchived(ctx, chat.ID, false)
	if err != nil || got.Pinned {
		t.Fatalf("SetArchived(false) = %+v, %v; want pinned false", got, err)
	}

	if len(f.organizer.pinned) != 0 {
		t.Errorf("organizer saw pins %v, want none: this chat was never pinned", f.organizer.pinned)
	}
}

// TestSetArchived_RestoresThePinLocallyEvenWhenUnarchivingTheServiceFails
// checks the pin still comes back here even when the service never
// confirms the unarchive itself, the same "local change always stands"
// rule SetArchived's own doc comment already states, and that no re-pin
// is even attempted with a service that never accepted the unarchive.
func TestSetArchived_RestoresThePinLocallyEvenWhenUnarchivingTheServiceFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.archiveUnpinner.unpins["wa"] = true

	if _, err := f.commands.SetArchived(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	f.organizer.err = errors.New("offline")
	got, err := f.commands.SetArchived(ctx, chat.ID, false)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("SetArchived(false) = %v, want ErrInvalidInput", err)
	}
	if !got.Pinned {
		t.Errorf("SetArchived(false) result = %+v, want pinned true despite the failed service call", got)
	}

	// Only the original pin was ever asked of the service: the failed
	// unarchive means there is nothing there to re-pin yet.
	if !slices.Equal(f.organizer.pinned, []string{"chat true"}) {
		t.Errorf("organizer saw pins %v, want only the original pin", f.organizer.pinned)
	}
}

// TestSetArchived_LogsARePinFailureSeparatelyFromTheUnarchive checks a
// re-pin that the service itself refuses, after its own unarchive
// already succeeded, is logged on its own, safe line rather than
// silently dropped or folded into the unarchive's own result.
func TestSetArchived_LogsARePinFailureSeparatelyFromTheUnarchive(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.archiveUnpinner.unpins["wa"] = true
	if _, err := f.commands.SetArchived(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.logger.take()

	f.organizer.pinErr = connector.ErrPinLimit
	got, err := f.commands.SetArchived(ctx, chat.ID, false)
	if err != nil {
		t.Fatalf("SetArchived(false) = %+v, %v, want the unarchive itself to succeed", got, err)
	}
	if !got.Pinned {
		t.Errorf("SetArchived(false) result = %+v, want pinned true locally regardless of the re-pin's own failure", got)
	}

	if lines := f.logger.take(); len(lines) != 1 || !strings.Contains(lines[0], "re-pin after unarchive failed") {
		t.Errorf("logged = %v, want one line naming the re-pin failure", lines)
	}
}

// TestSetArchived_LeavesAPinAloneForAServiceThatDoesNotRequireUnpinning
// confirms Telegram's own kind of service, whose own connector does not
// implement ArchiveUnpinner, keeps a locally pinned chat pinned once it
// is archived: Telegram keeps a separate pinned list per archive
// folder, so there is nothing here to correct.
func TestSetArchived_LeavesAPinAloneForAServiceThatDoesNotRequireUnpinning(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, err := f.commands.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.published.take()

	got, err := f.commands.SetArchived(ctx, chat.ID, true)
	if err != nil || !got.Pinned || !got.Archived {
		t.Fatalf("SetArchived = %+v, %v, want pinned true and archived true", got, err)
	}

	if !slices.Contains(f.archiveUnpinner.accounts, "wa") {
		t.Errorf("archiveUnpinner asked = %v, want it asked about wa", f.archiveUnpinner.accounts)
	}
}

// TestSetArchived_ReportsAServiceFailureAsSafeInvalidInput checks the
// fix for a report where archiving, then unarchiving, a chat whose
// service rejected the change surfaced only as the generic "internal
// error" banner, with nothing in the helper's own log saying why: any
// failure from the organizer here, once the local change already
// stands, must become a safe message naming what happened instead of
// the fixed internal error a request that changed nothing at all gets.
func TestSetArchived_ReportsAServiceFailureAsSafeInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.organizer.err = errors.New("offline")

	_, err := f.commands.SetArchived(ctx, chat.ID, true)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetArchived with a failing service = %v, want ErrInvalidInput", err)
	}
	if err != nil && !strings.Contains(err.Error(), "archived here") {
		t.Errorf("SetArchived error = %q, want it to say the change was made here", err.Error())
	}

	if got, err := f.store.Conversation(ctx, chat.ID); err != nil || !got.Archived {
		t.Errorf("Conversation after a failed SetArchived = %+v, %v; want Archived true", got, err)
	}

	if lines := f.logger.take(); len(lines) != 1 || !strings.Contains(lines[0], "set archived failed") {
		t.Errorf("logged = %v, want one line naming the failure", lines)
	}

	// Unarchiving a failure wording says "unarchived", not "archived".
	f.organizer.err = errors.New("offline again")
	_, err = f.commands.SetArchived(ctx, chat.ID, false)
	if err == nil || !strings.Contains(err.Error(), "unarchived here") {
		t.Errorf("SetArchived(false) with a failing service = %v, want it to say unarchived", err)
	}
}

// TestSetArchived_LogsThePreciseFailureClass checks organizeFailureClass
// through the public API: a connector's own ErrOrganizeSyncIncomplete
// (its local sync state was not ready yet, such as WhatsApp's app-state
// keys never finishing after pairing) and ErrOrganizeRejected (the
// service explicitly refused) each get their own named class in the
// log, distinct from "other", so a real account's report can be
// checked against which of the two it was without reading connector
// code.
func TestSetArchived_LogsThePreciseFailureClass(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		err  error
		want string
	}{
		"a connector's own sync-incomplete error": {err: connector.ErrOrganizeSyncIncomplete, want: "error_class=sync"},
		"a connector's own rejection":             {err: connector.ErrOrganizeRejected, want: "error_class=rejected"},
		"a context deadline":                      {err: context.DeadlineExceeded, want: "error_class=timeout"},
		"an error this package cannot name":       {err: errors.New("offline"), want: "error_class=other"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
			f.organizer.err = tt.err

			if _, err := f.commands.SetArchived(ctx, chat.ID, true); !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("SetArchived = %v, want ErrInvalidInput", err)
			}

			if lines := f.logger.take(); len(lines) != 1 || !strings.Contains(lines[0], tt.want) {
				t.Errorf("logged = %v, want a line containing %q", lines, tt.want)
			}
		})
	}
}
