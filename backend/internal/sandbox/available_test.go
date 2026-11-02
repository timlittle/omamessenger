package sandbox_test

// available_test.go tests Runner.Available black-box. The two failure
// cases below set PATH or name a binary so that bwrap, prlimit or the
// binary itself cannot be found, which works whether or not this
// machine actually has bwrap and prlimit installed; only the smoke-
// test success case needs the real pipeline, so only it skips.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

func TestAvailable_MissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty directory: bwrap and prlimit cannot resolve

	r := sandbox.NewRunner(t.TempDir())

	err := r.Available(t.Context(), "/usr/bin/true")
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, sandbox.ErrUnavailable)
	}
}

func TestAvailable_BinaryMissing(t *testing.T) {
	t.Parallel()

	r := sandbox.NewRunner(t.TempDir())

	err := r.Available(t.Context(), "/no/such/binary-for-this-test")
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, sandbox.ErrUnavailable)
	}
}

func TestAvailable_BinaryIsADirectory(t *testing.T) {
	t.Parallel()

	r := sandbox.NewRunner(t.TempDir())

	err := r.Available(t.Context(), t.TempDir())
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, sandbox.ErrUnavailable)
	}
}

func TestAvailable_BinaryNotExecutable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	r := sandbox.NewRunner(t.TempDir())

	err := r.Available(t.Context(), path)
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
