package server_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/server"
)

// writeSessionFile creates dir (and any missing parent) and writes a
// small real file at dir/name, for a files.open test whose path must
// survive filepath.EvalSymlinks rather than merely look plausible.
func writeSessionFile(t *testing.T, dir, name string) string {
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

// TestLinksOpen_OpensAnAllowedURL confirms links.open reaches
// app.Commands.OpenLink and carries the url through unchanged.
func TestLinksOpen_OpensAnAllowedURL(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[struct{}](t, s, "links.open", map[string]string{"url": "https://example.com/reset?token=secret"}); err != nil {
		t.Fatalf("links.open = %v", err)
	}
	if s.opener.gotURI != "https://example.com/reset?token=secret" {
		t.Errorf("Open was given %q, want the url passed through unchanged", s.opener.gotURI)
	}
}

// TestLinksOpen_RejectsADisallowedSchemeAsInvalidParams confirms a
// scheme outside the allow-list reaches the UI as invalid params, not
// an internal error, and never reaches the opener.
func TestLinksOpen_RejectsADisallowedSchemeAsInvalidParams(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	_, err := call[struct{}](t, s, "links.open", map[string]string{"url": "javascript:alert(1)"})
	if code(err) != server.CodeInvalidParams {
		t.Fatalf("links.open with a disallowed scheme code = %d, want invalid params", code(err))
	}
	if s.opener.gotURI != "" {
		t.Errorf("Open was called with %q despite the disallowed scheme", s.opener.gotURI)
	}
}

// TestLinksOpen_CarriesAnUnavailableReasonWhenThePortalCannotBeReached
// confirms an opener failure reaches the UI as an internal error whose
// data names reason "unavailable", the UI's own cue to show a safe
// message rather than falling back to xdg-open's own argv leak (see
// docs/decisions.md).
func TestLinksOpen_CarriesAnUnavailableReasonWhenThePortalCannotBeReached(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.opener.openErr = errPortalUnreachable{}

	_, err := call[struct{}](t, s, "links.open", map[string]string{"url": "https://example.com"})
	if code(err) != server.CodeInternal {
		t.Fatalf("links.open with no portal reachable code = %d, want internal error", code(err))
	}

	data, ok := errorData[struct {
		Reason string `json:"reason"`
	}](err)
	if !ok || data.Reason != "unavailable" {
		t.Errorf("error data = %+v, ok=%v, want reason %q", data, ok, "unavailable")
	}
}

// TestFilesOpen_OpensAFileInsideTheMediaCache confirms files.open
// reaches app.Commands.OpenFile and carries its resolved path through.
func TestFilesOpen_OpensAFileInsideTheMediaCache(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := writeSessionFile(t, s.mediaDir, "abc.jpg")

	if _, err := call[struct{}](t, s, "files.open", map[string]string{"path": path}); err != nil {
		t.Fatalf("files.open = %v", err)
	}
	if s.opener.gotPath != path {
		t.Errorf("OpenFile was given %q, want %q", s.opener.gotPath, path)
	}
}

// TestFilesOpen_OpensTheKeysConfigFile confirms the one file outside
// every cache or downloads directory files.open still opens.
func TestFilesOpen_OpensTheKeysConfigFile(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := writeSessionFile(t, filepath.Dir(s.keysConfigPath), filepath.Base(s.keysConfigPath))

	if _, err := call[struct{}](t, s, "files.open", map[string]string{"path": path}); err != nil {
		t.Fatalf("files.open = %v", err)
	}
	if s.opener.gotPath != path {
		t.Errorf("OpenFile was given %q, want %q", s.opener.gotPath, path)
	}
}

// TestFilesOpen_RejectsAnEmptyPathAsInvalidParams confirms an empty
// path reaches the UI as invalid params, not an internal error.
func TestFilesOpen_RejectsAnEmptyPathAsInvalidParams(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	_, err := call[struct{}](t, s, "files.open", map[string]string{"path": ""})
	if code(err) != server.CodeInvalidParams {
		t.Fatalf("files.open with an empty path code = %d, want invalid params", code(err))
	}
}

// TestFilesOpen_RejectsAPathOutsideEveryAllowedRootAsInvalidParams
// confirms a real file that is simply not one of this helper's own
// cache, downloads or key-bindings locations is refused, never handed
// to the opener.
func TestFilesOpen_RejectsAPathOutsideEveryAllowedRootAsInvalidParams(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := writeSessionFile(t, t.TempDir(), "sessions.db")

	_, err := call[struct{}](t, s, "files.open", map[string]string{"path": path})
	if code(err) != server.CodeInvalidParams {
		t.Fatalf("files.open outside every allowed root code = %d, want invalid params", code(err))
	}
	if s.opener.gotPath != "" {
		t.Errorf("OpenFile reached the opener with %q despite the path lying outside every allowed root", s.opener.gotPath)
	}
}

// TestFilesOpen_RejectsASymlinkInsideTheCacheThatEscapesIt reproduces
// a symlink planted inside the media cache whose own target lies
// outside every allowed root: the request's own path looks safe, but
// filepath.EvalSymlinks must still catch where it actually leads.
func TestFilesOpen_RejectsASymlinkInsideTheCacheThatEscapesIt(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	outside := writeSessionFile(t, t.TempDir(), "secret.txt")

	if err := os.MkdirAll(s.mediaDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(s.mediaDir, "innocent.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := call[struct{}](t, s, "files.open", map[string]string{"path": link})
	if code(err) != server.CodeInvalidParams {
		t.Fatalf("files.open over an escaping symlink code = %d, want invalid params", code(err))
	}
	if s.opener.gotPath != "" {
		t.Errorf("OpenFile followed a symlink out of the media cache and reached the opener with %q", s.opener.gotPath)
	}
}

// errPortalUnreachable is a plain error standing in for the portal's
// own dial or Open/OpenFile failure; its text never needs to cross the
// protocol, only app.OpenError's fixed "unavailable" category does.
type errPortalUnreachable struct{}

func (errPortalUnreachable) Error() string { return "no portal running" }
