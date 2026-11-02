package sandbox_test

// sandbox_test.go runs real jobs through the real bwrap, prlimit and
// seccomp pipeline, black-box, through Runner's own public API. Every
// test here skips rather than fails on a machine that cannot actually
// run it (see requireSandbox in helpers_test.go); process_internal_
// test.go covers the same start/wait/kill/classify logic with a fake
// command so it still runs - and still counts toward this package's
// coverage gate - everywhere else.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

func TestRun_RealTrivialCommandSucceeds(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/true")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	result, err := r.Run(ctx, sandbox.Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Succeeded() {
		t.Fatalf("Run() = %+v, want Succeeded() true", result)
	}
}

func TestRun_NetworkUnreachable(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/bash")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spec := sandbox.Spec{
		Binary:    "/usr/bin/bash",
		Args:      []string{"-c", "exec 3<>/dev/tcp/1.1.1.1/80"},
		OutputDir: t.TempDir(),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded() {
		t.Fatal("Run() succeeded trying to open a TCP connection; the sandbox's network is not actually unshared")
	}
}

func TestRun_FileOutsideBoundPathsUnreadable(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/cat")

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("not for the sandbox"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spec := sandbox.Spec{
		Binary:    "/usr/bin/cat",
		Args:      []string{outside},
		OutputDir: t.TempDir(),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded() {
		t.Fatal("Run() read a host file outside Input/OutputDir; it should not have been visible inside the sandbox at all")
	}
}

func TestRun_SeccompDeniesUnshare(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/unshare")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spec := sandbox.Spec{
		Binary:    "/usr/bin/unshare",
		Args:      []string{"-U", "/usr/bin/true"},
		OutputDir: t.TempDir(),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// bwrap's own monitor reports a signal-killed sandboxed command
	// through its own ordinary exit, using the common "128+signal"
	// shell convention (confirmed on this machine: unshare's own
	// SIGSYS kill, signal 31, surfaces as exit code 159), rather than
	// the outer process this package waits on being signalled itself -
	// so this checks that the job failed at all, not the exact code.
	if result.Succeeded() {
		t.Fatal("Run() succeeded calling unshare; the seccomp filter did not deny it")
	}
	if result.TimedOut || result.TooLarge {
		t.Fatalf("Run() = %+v, want an ordinary failure, not a timeout or an oversized output", result)
	}
}
