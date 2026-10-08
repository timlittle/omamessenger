package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/doctor"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// doctorReport runs f.commands.Doctor, failing the test on an unexpected
// error.
func doctorReport(t *testing.T, f *fixture) doctor.Report {
	t.Helper()

	report, err := f.commands.Doctor(t.Context())
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	return report
}

// checkNamed finds the one check named name, failing the test if there is
// not exactly one.
func checkNamed(t *testing.T, r doctor.Report, name string) doctor.Check {
	t.Helper()

	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}

	t.Fatalf("no check named %q among %+v", name, r.Checks)

	return doctor.Check{}
}

// doctorCommandsWithPaths builds a standalone Commands over its own
// throwaway database, naming dataDir and dbPath for the permission
// check, without touching the shared fixture.
func doctorCommandsWithPaths(t *testing.T, dataDir, dbPath string) *app.Commands {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	commands, _ := app.New(app.Deps{Store: db, DataDir: dataDir, DBPath: dbPath})

	return commands
}

func TestDoctor_FlagsLoosePermissions(t *testing.T) {
	t.Parallel()

	secureDir := t.TempDir()
	if err := os.Chmod(secureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secureFile := filepath.Join(secureDir, "messages.db")
	if err := os.WriteFile(secureFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	secure := doctorCommandsWithPaths(t, secureDir, secureFile)
	report, err := secure.Doctor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := checkNamed(t, report, "Data permissions"); !got.OK {
		t.Errorf("secure data dir and database file = %+v, want ok", got)
	}

	looseFile := filepath.Join(secureDir, "loose.db")
	if err := os.WriteFile(looseFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	loose := doctorCommandsWithPaths(t, secureDir, looseFile)
	report, err = loose.Doctor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := checkNamed(t, report, "Data permissions"); got.OK {
		t.Errorf("a database file readable by others = %+v, want a problem", got)
	}
}

func TestDoctor_ReportsOneCheckPerAccountByItsStatus(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()

	if _, err := f.store.SetAccountStatus(ctx, "wa", domain.AccountError, ""); err != nil {
		t.Fatal(err)
	}

	got := checkNamed(t, doctorReport(t, f), "Account (whatsapp)")
	if got.OK || got.Detail != "error" {
		t.Errorf("errored account = %+v, want a problem detailed \"error\"", got)
	}
}

func TestDoctor_ChecksTheCacheAgainstItsLimitAndTheDatabaseOpened(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	report := doctorReport(t, f)

	if got := checkNamed(t, report, "Media cache"); !got.OK {
		t.Errorf("an empty cache = %+v, want within its limit", got)
	}
	if got := checkNamed(t, report, "Database"); !got.OK {
		t.Errorf("a working store = %+v, want open and up to date", got)
	}
	if got := checkNamed(t, report, "Outgoing attachments"); !got.OK {
		t.Errorf("an empty outgoing area = %+v, want within its limit", got)
	}
}

// TestDoctor_FlagsOutgoingAttachmentsOverTheirLimit confirms Doctor
// reports the outgoing area's own check as a problem once it holds more
// than the limit it was given, independent of the downloaded media
// cache's own check.
func TestDoctor_FlagsOutgoingAttachmentsOverTheirLimit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	outgoing := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 10)
	if _, err := outgoing.Store(ctx, "m1", "file.bin", strings.NewReader("this is over ten bytes")); err != nil {
		t.Fatal(err)
	}

	commands, _ := app.New(app.Deps{Store: db, Outgoing: outgoing})
	report, err := commands.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if got := checkNamed(t, report, "Outgoing attachments"); got.OK {
		t.Errorf("an oversized outgoing area = %+v, want a problem", got)
	}
}

func TestDoctor_RecordsTheSafeCategoryOfARecentMediaFetchFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	f.media.err = domain.ErrMediaExpired
	if _, err := f.commands.FetchMedia(ctx, "m1"); err == nil {
		t.Fatal("FetchMedia with a failing connector succeeded")
	}

	if got := checkNamed(t, doctorReport(t, f), "Recent errors"); got.Detail != "expired" {
		t.Errorf("Recent errors detail = %q, want %q", got.Detail, "expired")
	}
}

// TestDoctor_VersionMatchesTheExecutableNamesPin confirms the fixture's
// own stand-in executable name and version agree, the common case.
func TestDoctor_VersionMatchesTheExecutableNamesPin(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	if got := checkNamed(t, doctorReport(t, f), "Helper version"); !got.OK {
		t.Errorf("matching version and executable name = %+v, want ok", got)
	}
}
