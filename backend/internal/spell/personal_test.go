package spell

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/client9/gospell"
)

// This file is an internal test: loadPersonalWords, appendPersonalWord
// and securePersonalDir are implementation details of Checker, and the
// specific OS-level errors below (a path component that is not a
// directory, a target that is a directory, loose permissions already
// set) are far more direct to trigger against these small helpers
// themselves than through Checker's own public methods.

// testDict returns a tiny GoSpell for tests that need one to add words
// to, not to check spelling against.
func testDict(t *testing.T) *gospell.GoSpell {
	t.Helper()

	dict, err := gospell.NewGoSpellReader(strings.NewReader(""), strings.NewReader("1\nhello\n"))
	if err != nil {
		t.Fatal(err)
	}

	return dict
}

// TestLoadPersonalWords_MissingFileIsNotAnError is the common case: no
// personal words have been added yet for this locale.
func TestLoadPersonalWords_MissingFileIsNotAnError(t *testing.T) {
	t.Parallel()

	if err := loadPersonalWords(filepath.Join(t.TempDir(), "missing.txt"), testDict(t)); err != nil {
		t.Errorf("loadPersonalWords(missing file) = %v, want nil", err)
	}
}

// TestLoadPersonalWords_OpenErrorOtherThanMissing reports an error for
// a path that cannot exist for a reason other than being missing, such
// as a parent component that is a plain file, not a directory.
func TestLoadPersonalWords_OpenErrorOtherThanMissing(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := loadPersonalWords(filepath.Join(parent, "en_US.txt"), testDict(t))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Errorf("loadPersonalWords(path under a file) = %v, want a non-not-exist error", err)
	}
}

// TestLoadPersonalWords_UnreadablePathReportsScannerError reports a
// scan failure, such as the path naming a directory instead of a file.
func TestLoadPersonalWords_UnreadablePathReportsScannerError(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "en_US.txt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := loadPersonalWords(dir, testDict(t)); err == nil {
		t.Error("loadPersonalWords(a directory) = nil error, want one")
	}
}

// TestAppendPersonalWord_OpenFileErrorWhenPathIsADirectory reports an
// error rather than silently doing nothing when the personal word list
// path is already a directory.
func TestAppendPersonalWord_OpenFileErrorWhenPathIsADirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "spelling")
	path := filepath.Join(dir, "en_US.txt")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := appendPersonalWord(path, "hello"); err == nil {
		t.Error("appendPersonalWord(path is a directory) = nil error, want one")
	}
}

// TestSecurePersonalDir_AlreadyPrivateLeavesItAlone returns without
// error, and without needing to chmod, when the directory already
// exists at 0700.
func TestSecurePersonalDir_AlreadyPrivateLeavesItAlone(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "spelling")
	if err := securePersonalDir(dir); err != nil {
		t.Fatalf("first securePersonalDir: %v", err)
	}

	if err := securePersonalDir(dir); err != nil {
		t.Errorf("second securePersonalDir(already 0700) = %v, want nil", err)
	}
}

// TestSecurePersonalDir_TightensLoosePermissions chmods an
// already-existing directory that is readable or writable by the group
// or others back down to 0700.
func TestSecurePersonalDir_TightensLoosePermissions(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "spelling")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := securePersonalDir(dir); err != nil {
		t.Fatalf("securePersonalDir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory mode = %o, want 0700", perm)
	}
}

// TestSecurePersonalDir_StatErrorOtherThanMissing reports an error for
// a directory that cannot be statted for a reason other than being
// missing, such as a parent component that is a plain file.
func TestSecurePersonalDir_StatErrorOtherThanMissing(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := securePersonalDir(filepath.Join(parent, "spelling"))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Errorf("securePersonalDir(path under a file) = %v, want a non-not-exist error", err)
	}
}
