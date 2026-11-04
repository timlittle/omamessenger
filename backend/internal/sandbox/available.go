package sandbox

// available.go checks, once per Runner, that this machine can actually
// run a job: bwrap and prlimit resolve on PATH, and a trivial command
// succeeds end to end through the real pipeline, not just that the
// files exist.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnavailable reports that this machine cannot run a sandboxed job
// right now: bwrap or prlimit is missing, a binary Available was asked
// about does not exist or is not executable, or the trivial sandboxed
// command Available itself runs failed.
var ErrUnavailable = errors.New("sandbox: unavailable")

// trueBinary is what Available's smoke test runs: present on every
// Omarchy system, needs no arguments, and exits 0 immediately.
const trueBinary = "/usr/bin/true"

// Available reports whether Run can actually sandbox binary on this
// machine. The expensive smoke test happens at most once per Runner
// and is cached; binary itself is checked fresh on every call.
func (r *Runner) Available(ctx context.Context, binary string) error {
	if err := r.resolveTools(); err != nil {
		return err
	}
	if err := checkExecutable(binary); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	r.smokeOnce.Do(func() {
		r.smokeErr = r.runSmokeTest(ctx)
	})

	return r.smokeErr
}

// checkExecutable reports an error unless path names a regular file
// with at least one executable bit set.
func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("sandbox: %s is not executable", path)
	}

	return nil
}

// runSmokeTest runs trueBinary through the real sandbox pipeline,
// proving bwrap, prlimit and this package's own seccomp filter work
// together on this machine right now.
func (r *Runner) runSmokeTest(ctx context.Context) error {
	dir, err := os.MkdirTemp(r.scratchRoot, "smoke-*")
	if err != nil {
		return fmt.Errorf("sandbox: smoke test: %w", err)
	}
	defer os.RemoveAll(dir) // best-effort: a throwaway directory this call alone created and never shares with anything else

	output := filepath.Join(dir, "out")
	if err := os.WriteFile(output, nil, 0o600); err != nil {
		return fmt.Errorf("sandbox: smoke test: %w", err)
	}

	result, err := r.Run(ctx, Spec{Binary: trueBinary, Output: output})
	if err != nil {
		return fmt.Errorf("sandbox: smoke test: %w", err)
	}
	if !result.Succeeded() {
		return fmt.Errorf("%w: smoke test: exit=%d timedOut=%v tooLarge=%v",
			ErrUnavailable, result.ExitCode, result.TimedOut, result.TooLarge)
	}

	return nil
}
