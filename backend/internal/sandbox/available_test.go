package sandbox_test

// available_test.go tests Runner.Available black-box. The failure
// case below sets PATH so that env, bwrap and prlimit cannot resolve
// at all, which works whether or not this machine actually has them
// installed; only the smoke-test success case needs the real
// pipeline, so only it skips. checkExecutable's own three branches,
// and the smoke test's own success return, are tested as an internal
// test instead (available_internal_test.go): reaching them needs
// resolveTools to succeed regardless of what is actually on this
// machine's PATH, which only the unexported lookPath seam can
// guarantee - on a CI runner with no bwrap installed, a black-box
// version using the real lookPath would never reach them at all,
// since resolveTools would already have failed first.

import (
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

func TestAvailable_MissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty directory: env, bwrap and prlimit cannot resolve

	r := sandbox.NewRunner(t.TempDir())

	err := r.Available(t.Context(), "/usr/bin/true")
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, sandbox.ErrUnavailable)
	}
}

func TestAvailable_RealSmokeTestSucceeds(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/true")

	if err := r.Available(t.Context(), "/usr/bin/true"); err != nil {
		t.Fatalf("Available() error = %v after requireSandbox already confirmed it works", err)
	}
}
