package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/portal"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// fakeFilePicker records the title and filters its Pick was given and
// returns whatever path, ok and err a test scripted, standing in for
// portal.Picker so no test ever opens a real file-choosing dialog over
// D-Bus. Its default zero value (ok false, err nil) behaves like a
// cancelled pick, harmless for any test that never configures it.
type fakeFilePicker struct {
	mu      sync.Mutex
	title   string
	filters []portal.Filter

	path string
	ok   bool
	err  error
}

// Pick records title and filters and returns the fake's scripted
// reply.
func (f *fakeFilePicker) Pick(_ context.Context, title string, filters []portal.Filter) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.title, f.filters = title, filters

	return f.path, f.ok, f.err
}

// lastPick returns the title and filters the most recent Pick call
// was given, under the fake's lock.
func (f *fakeFilePicker) lastPick() (string, []portal.Filter) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.title, f.filters
}

func TestPickFile_ReturnsThePathOnASuccessfulPick(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.filePicker.path, f.filePicker.ok = "/home/tim/vacation.jpg", true

	path, err := f.commands.PickFile(t.Context(), "attach", "Attach a file")
	if err != nil {
		t.Fatalf("PickFile: %v", err)
	}
	if path != "/home/tim/vacation.jpg" {
		t.Errorf("path = %q, want /home/tim/vacation.jpg", path)
	}

	gotTitle, gotFilters := f.filePicker.lastPick()
	if gotTitle != "Attach a file" {
		t.Errorf("title = %q, want %q", gotTitle, "Attach a file")
	}
	if len(gotFilters) == 0 {
		t.Error("filters = none, want the attach purpose's own filters")
	}
}

// TestPickFile_FallsBackToAFixedTitle confirms an empty title (the
// server's own params carry none) still names the dialog something,
// rather than showing it with no title at all.
func TestPickFile_FallsBackToAFixedTitle(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.filePicker.ok = true

	if _, err := f.commands.PickFile(t.Context(), "attach", ""); err != nil {
		t.Fatalf("PickFile: %v", err)
	}

	gotTitle, _ := f.filePicker.lastPick()
	if gotTitle == "" {
		t.Error("title = \"\", want a fixed fallback title")
	}
}

// TestPickFile_UsesTheSoundPurposesOwnFilters confirms "sound" and
// "attach" offer different filters, so the sound picker is not left
// showing every file kind the attach button does.
func TestPickFile_UsesTheSoundPurposesOwnFilters(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.filePicker.ok = true

	if _, err := f.commands.PickFile(t.Context(), "sound", "Set notification sound"); err != nil {
		t.Fatalf("PickFile: %v", err)
	}

	_, soundGot := f.filePicker.lastPick()

	f2 := newFixture(t, false)
	f2.filePicker.ok = true
	if _, err := f2.commands.PickFile(t.Context(), "attach", "Attach a file"); err != nil {
		t.Fatalf("PickFile: %v", err)
	}
	_, attachGot := f2.filePicker.lastPick()

	if len(soundGot) == 0 || len(attachGot) == 0 {
		t.Fatal("both purposes should offer at least one filter")
	}
	if soundGot[0].Name == attachGot[0].Name {
		t.Errorf("sound and attach offered the same filter %q, want purpose-specific filters", soundGot[0].Name)
	}
}

// TestPickFile_ReturnsNoPathAndNoErrorWhenCancelled confirms a
// cancelled pick (ok false, err nil from the FilePicker) is never
// reported as a failure: there is nothing for the UI to fall back to
// or show an error for.
func TestPickFile_ReturnsNoPathAndNoErrorWhenCancelled(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.filePicker.ok, f.filePicker.path = false, ""

	path, err := f.commands.PickFile(t.Context(), "attach", "Attach a file")
	if err != nil {
		t.Fatalf("PickFile over a cancelled pick returned an error: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q, want empty once the user cancelled", path)
	}
}

// TestPickFile_FailsWithAnUnavailableReasonWhenTheDialogCannotShow
// confirms a FilePicker failure (the portal unreachable, say) becomes
// a *FilePickError naming reason "unavailable": the UI's own cue to
// fall back to a different file picker, never to the in-process GTK
// dialog that crashed the whole shell (see docs/decisions.md).
func TestPickFile_FailsWithAnUnavailableReasonWhenTheDialogCannotShow(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	cause := errors.New("no portal running")
	f.filePicker.err = cause

	_, err := f.commands.PickFile(t.Context(), "attach", "Attach a file")

	var pickErr *app.FilePickError
	if !errors.As(err, &pickErr) {
		t.Fatalf("err = %v, want a *app.FilePickError", err)
	}
	if pickErr.Reason != "unavailable" {
		t.Errorf("reason = %q, want %q", pickErr.Reason, "unavailable")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is did not see through FilePickError to its underlying cause")
	}
}

// TestPickFile_FailsWithAnUnavailableReasonWhenNoFilePickerIsWired
// confirms a helper built with no FilePicker at all (Deps.FilePicker
// left nil) fails the same safe way as one whose dialog itself failed,
// rather than panicking on a nil interface.
func TestPickFile_FailsWithAnUnavailableReasonWhenNoFilePickerIsWired(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "messages.db")
	db, err := store.Open(t.Context(), dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	commands, _ := app.New(app.Deps{Store: db})

	_, err = commands.PickFile(t.Context(), "attach", "Attach a file")

	var pickErr *app.FilePickError
	if !errors.As(err, &pickErr) {
		t.Fatalf("err = %v, want a *app.FilePickError", err)
	}
	if pickErr.Reason != "unavailable" {
		t.Errorf("reason = %q, want %q", pickErr.Reason, "unavailable")
	}
}
