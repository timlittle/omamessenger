package transcode

// jobinput_test.go tests placeJobInput and copyJobInput directly, as
// an internal test: run's own public API already exercises
// placeJobInput's ordinary hard-link path (see TestVideo_Succeeds in
// transcode_test.go), but forcing its copy fallback through the
// public API would need a genuine cross-filesystem setup this suite
// cannot construct hermetically, and proving the oversized-input
// refusal runs before any byte is read is far cheaper against a
// sparse file than a real multi-hundred-megabyte fixture would be.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

func TestPlaceJobInput_Links(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "attachment-original-name.mp4")
	if err := os.WriteFile(src, []byte("original bytes"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	jobDir := filepath.Join(dir, "job")
	if err := os.Mkdir(jobDir, 0o700); err != nil {
		t.Fatalf("making job directory: %v", err)
	}

	got, err := placeJobInput(src, jobDir)
	if err != nil {
		t.Fatalf("placeJobInput() error = %v", err)
	}

	if want := filepath.Join(jobDir, sandbox.InputBaseName); got != want {
		t.Fatalf("placeJobInput() = %q, want %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading placed input: %v", err)
	}
	if string(data) != "original bytes" {
		t.Fatalf("placed input = %q, want %q", data, "original bytes")
	}
}

func TestPlaceJobInput_MissingSource(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	_, err := placeJobInput(filepath.Join(t.TempDir(), "never-written"), jobDir)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("placeJobInput() error = %v, want %v", err, ErrFailed)
	}
}

func TestCopyJobInput_CopiesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("copy me"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	dst := filepath.Join(dir, "dst")

	if err := copyJobInput(src, dst); err != nil {
		t.Fatalf("copyJobInput() error = %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading copy: %v", err)
	}
	if string(data) != "copy me" {
		t.Fatalf("copy = %q, want %q", data, "copy me")
	}
}

func TestCopyJobInput_MissingSource(t *testing.T) {
	t.Parallel()

	err := copyJobInput(filepath.Join(t.TempDir(), "never-written"), filepath.Join(t.TempDir(), "dst"))
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("copyJobInput() error = %v, want %v", err, ErrFailed)
	}
}

func TestCopyJobInput_RefusesOversizedInputBeforeReading(t *testing.T) {
	t.Parallel()

	// A sparse file reports its full, declared size through Stat
	// without this test actually writing - or this package actually
	// having to read - anywhere near maxJobInputCopyBytes of real
	// data: proving the cap is checked before copyJobInput ever opens
	// the file for reading, not after a partial copy.
	dir := t.TempDir()
	src := filepath.Join(dir, "huge")
	f, err := os.Create(src)
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	if err := f.Truncate(maxJobInputCopyBytes + 1); err != nil {
		t.Fatalf("truncating fixture to a sparse size: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing fixture: %v", err)
	}

	err = copyJobInput(src, filepath.Join(dir, "dst"))
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("copyJobInput() error = %v, want %v", err, ErrFailed)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "dst")); !os.IsNotExist(statErr) {
		t.Fatal("copyJobInput() left a destination file behind after refusing an oversized input")
	}
}

func TestCopyJobInput_DestinationAlreadyExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(dst, []byte("already here"), 0o600); err != nil {
		t.Fatalf("pre-creating destination: %v", err)
	}

	// O_EXCL refuses to create a file that is already there, the same
	// guard writeJobInputCopy relies on to never silently overwrite
	// whatever is already at the job's own fixed input path.
	err := copyJobInput(src, dst)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("copyJobInput() error = %v, want %v", err, ErrFailed)
	}
}
