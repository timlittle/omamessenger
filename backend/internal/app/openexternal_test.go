package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// writeFixtureFile creates dir (and any missing parent) and writes a
// small real file at dir/name, for a test that needs OpenFile's own
// filepath.EvalSymlinks to find something real rather than a path
// that merely looks plausible.
func writeFixtureFile(t *testing.T, dir, name string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

// fakeOpener records what its Open and OpenFile were given and returns
// whatever error a test scripted, standing in for portal.Opener so no
// test ever reaches a real portal over D-Bus. Its default zero value
// (no error) behaves like a successful open.
type fakeOpener struct {
	mu sync.Mutex

	gotURI  string
	gotPath string

	openErr     error
	openFileErr error
}

// Open records uri and returns the fake's scripted error.
func (f *fakeOpener) Open(_ context.Context, uri string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gotURI = uri

	return f.openErr
}

// OpenFile records path and returns the fake's scripted error.
func (f *fakeOpener) OpenFile(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gotPath = path

	return f.openFileErr
}

// lastURI returns the most recent Open call's own uri, under the
// fake's lock.
func (f *fakeOpener) lastURI() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.gotURI
}

// lastPath returns the most recent OpenFile call's own path, under the
// fake's lock.
func (f *fakeOpener) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.gotPath
}

func TestOpenLink_OpensAnAllowedURL(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.OpenLink(t.Context(), "https://example.com/reset?token=secret"); err != nil {
		t.Fatalf("OpenLink: %v", err)
	}
	if got := f.opener.lastURI(); got != "https://example.com/reset?token=secret" {
		t.Errorf("Open was given %q, want the url passed through unchanged", got)
	}
}

// TestOpenLink_RefusesADisallowedScheme confirms a scheme outside the
// allow-list never reaches the opener at all, re-applying the same
// check the UI already does rather than trusting it alone (see
// docs/decisions.md).
func TestOpenLink_RefusesADisallowedScheme(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	err := f.commands.OpenLink(t.Context(), "javascript:alert(1)")
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want app.ErrInvalidInput", err)
	}
	if got := f.opener.lastURI(); got != "" {
		t.Errorf("Open was called with %q despite the disallowed scheme", got)
	}
}

// TestOpenLink_FailsWithAnUnavailableReasonWhenTheOpenerFails confirms
// an URIOpener failure (the portal unreachable, say) becomes an
// *app.OpenError naming reason "unavailable", the UI's own cue to
// show a safe message rather than falling back to xdg-open's own argv
// leak (see docs/decisions.md).
func TestOpenLink_FailsWithAnUnavailableReasonWhenTheOpenerFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	cause := errors.New("no portal running")
	f.opener.openErr = cause

	err := f.commands.OpenLink(t.Context(), "https://example.com")

	var openErr *app.OpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("err = %v, want a *app.OpenError", err)
	}
	if openErr.Reason != "unavailable" {
		t.Errorf("reason = %q, want %q", openErr.Reason, "unavailable")
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is did not see through OpenError to its underlying cause")
	}
}

// TestOpenLink_FailsWithAnUnavailableReasonWhenNoOpenerIsWired
// confirms a helper built with no URIOpener at all (Deps.Opener left
// nil) fails the same safe way as one whose open itself failed, rather
// than panicking on a nil interface.
func TestOpenLink_FailsWithAnUnavailableReasonWhenNoOpenerIsWired(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "messages.db")
	db, err := store.Open(t.Context(), dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	commands, _ := app.New(app.Deps{Store: db})

	err = commands.OpenLink(t.Context(), "https://example.com")

	var openErr *app.OpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("err = %v, want a *app.OpenError", err)
	}
	if openErr.Reason != "unavailable" {
		t.Errorf("reason = %q, want %q", openErr.Reason, "unavailable")
	}
}

func TestOpenFile_OpensAFileInsideTheMediaCache(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeFixtureFile(t, f.mediaDir, "abc.jpg")

	if err := f.commands.OpenFile(t.Context(), path); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := f.opener.lastPath(); got != path {
		t.Errorf("OpenFile was given %q, want %q", got, path)
	}
}

// TestOpenFile_OpensAFileInsideTheOutgoingArea confirms a user's own
// staged attachment, in the outgoing area rather than the downloaded
// media cache, is allowed too.
func TestOpenFile_OpensAFileInsideTheOutgoingArea(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeFixtureFile(t, f.outgoingDir, "draft.jpg")

	if err := f.commands.OpenFile(t.Context(), path); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := f.opener.lastPath(); got != path {
		t.Errorf("OpenFile was given %q, want %q", got, path)
	}
}

// TestOpenFile_OpensAFileInsideDownloads confirms a file media.save
// already copied into the user's own downloads directory is allowed.
func TestOpenFile_OpensAFileInsideDownloads(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeFixtureFile(t, f.downloads, "vacation.jpg")

	if err := f.commands.OpenFile(t.Context(), path); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := f.opener.lastPath(); got != path {
		t.Errorf("OpenFile was given %q, want %q", got, path)
	}
}

// TestOpenFile_OpensTheKeysConfigFile confirms the one file outside
// every cache or downloads directory this helper will still open: its
// own fixed key-bindings file.
func TestOpenFile_OpensTheKeysConfigFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	if err := os.MkdirAll(filepath.Dir(f.keysConfigPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(f.keysConfigPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("write keys.conf: %v", err)
	}

	if err := f.commands.OpenFile(t.Context(), f.keysConfigPath); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := f.opener.lastPath(); got != f.keysConfigPath {
		t.Errorf("OpenFile was given %q, want %q", got, f.keysConfigPath)
	}
}

func TestOpenFile_RefusesAnEmptyPath(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.OpenFile(t.Context(), ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want app.ErrInvalidInput", err)
	}
}

// TestOpenFile_RefusesAPathThatDoesNotExist confirms a path
// filepath.EvalSymlinks cannot resolve at all - never downloaded,
// never saved, never real - fails the same safe way as any other
// disallowed path.
func TestOpenFile_RefusesAPathThatDoesNotExist(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	err := f.commands.OpenFile(t.Context(), filepath.Join(f.mediaDir, "missing.jpg"))
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want app.ErrInvalidInput", err)
	}
	if got := f.opener.lastPath(); got != "" {
		t.Errorf("OpenFile reached the opener with %q despite the missing file", got)
	}
}

// TestOpenFile_RefusesAPathOutsideEveryAllowedRoot is the finding this
// closes: a files.open call naming a path that is real, but nowhere
// this helper itself keeps a file, such as the account's own session
// database or an arbitrary file elsewhere on disk.
func TestOpenFile_RefusesAPathOutsideEveryAllowedRoot(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeFixtureFile(t, t.TempDir(), "sessions.db")

	err := f.commands.OpenFile(t.Context(), path)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want app.ErrInvalidInput", err)
	}
	if got := f.opener.lastPath(); got != "" {
		t.Errorf("OpenFile reached the opener with %q despite the path lying outside every allowed root", got)
	}
}

// TestOpenFile_RefusesASymlinkInsideTheCacheThatEscapesIt reproduces a
// symlink planted inside an otherwise-trusted directory (the media
// cache has never let this helper's own downloads contain one, but
// OpenFile must not assume that): the link's own path lies inside
// mediaDir, yet filepath.EvalSymlinks follows it to a file this helper
// never put there at all, which must still be refused.
func TestOpenFile_RefusesASymlinkInsideTheCacheThatEscapesIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	outside := writeFixtureFile(t, t.TempDir(), "secret.txt")

	if err := os.MkdirAll(f.mediaDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(f.mediaDir, "innocent.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := f.commands.OpenFile(t.Context(), link)
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want app.ErrInvalidInput", err)
	}
	if got := f.opener.lastPath(); got != "" {
		t.Errorf("OpenFile followed a symlink out of the media cache and reached the opener with %q", got)
	}
}

func TestOpenFile_FailsWithAnUnavailableReasonWhenTheOpenerFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeFixtureFile(t, f.mediaDir, "abc.jpg")
	cause := errors.New("no portal running")
	f.opener.openFileErr = cause

	err := f.commands.OpenFile(t.Context(), path)

	var openErr *app.OpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("err = %v, want a *app.OpenError", err)
	}
	if openErr.Reason != "unavailable" {
		t.Errorf("reason = %q, want %q", openErr.Reason, "unavailable")
	}
}
