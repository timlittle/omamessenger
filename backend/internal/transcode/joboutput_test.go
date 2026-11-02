package transcode

// joboutput_test.go tests createJobOutput directly, as an internal
// test: run's own public API already exercises its ordinary path (see
// TestVideo_Succeeds in transcode_test.go), but the already-taken-name
// case needs direct access to jobDir's own exact layout, which the
// public API never exposes.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateJobOutput_CreatesAnEmptyFile(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()

	got, err := createJobOutput(jobDir, "out.mp4")
	if err != nil {
		t.Fatalf("createJobOutput() error = %v", err)
	}

	if want := filepath.Join(jobDir, "out.mp4"); got != want {
		t.Fatalf("createJobOutput() = %q, want %q", got, want)
	}

	info, err := os.Lstat(got)
	if err != nil {
		t.Fatalf("statting created output: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("created output mode = %v, want a regular file", info.Mode())
	}
	if info.Size() != 0 {
		t.Fatalf("created output size = %d, want 0", info.Size())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("created output permissions = %o, want 0600", perm)
	}
}

func TestCreateJobOutput_NameAlreadyTaken(t *testing.T) {
	t.Parallel()

	// jobDir is always freshly made by os.MkdirTemp for exactly one
	// job (see run), so this should never actually happen; createJobOutput
	// still has to fail rather than silently reuse - and possibly
	// truncate - whatever is already there if it somehow does.
	jobDir := t.TempDir()
	existing := filepath.Join(jobDir, "out.mp4")
	if err := os.WriteFile(existing, []byte("already here"), 0o600); err != nil {
		t.Fatalf("pre-creating fixture: %v", err)
	}

	_, err := createJobOutput(jobDir, "out.mp4")
	if err == nil {
		t.Fatal("createJobOutput() error = nil, want an error for a name that already exists")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("createJobOutput() error = %v, want it to wrap os.ErrExist", err)
	}

	data, readErr := os.ReadFile(existing)
	if readErr != nil {
		t.Fatalf("reading existing file back: %v", readErr)
	}
	if string(data) != "already here" {
		t.Fatalf("existing file changed to %q, want it left untouched", data)
	}
}
