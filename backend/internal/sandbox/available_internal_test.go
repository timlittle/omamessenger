package sandbox

// available_internal_test.go tests checkExecutable's own three
// branches, and runSmokeTest's success return, as an internal test:
// each needs resolveTools to succeed so Available actually reaches
// the code under test, which the public, black-box equivalents in
// available_test.go cannot guarantee - they use the real lookPath, so
// on a machine (or a CI runner) with no bwrap installed, resolveTools
// itself already fails first, and these branches are never reached at
// all. fakeRunner (process_internal_test.go's own helper) makes
// resolveTools succeed regardless of what is actually on PATH, so
// these are reached every time, everywhere.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAvailable_BinaryMissing(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 0")

	err := r.Available(t.Context(), "/no/such/binary-for-this-test")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, ErrUnavailable)
	}
}

func TestAvailable_BinaryIsADirectory(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 0")

	err := r.Available(t.Context(), t.TempDir())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, ErrUnavailable)
	}
}

func TestAvailable_BinaryNotExecutable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	r := fakeRunner(t, "exit 0")

	err := r.Available(t.Context(), path)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, ErrUnavailable)
	}
}

func TestAvailable_SmokeTestSucceeds(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 0")

	if err := r.Available(t.Context(), "/usr/bin/true"); err != nil {
		t.Fatalf("Available() error = %v, want nil", err)
	}
}
