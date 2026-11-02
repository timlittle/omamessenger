package sandbox

// available.go checks, once per Runner, that this machine can actually
// run a job: bwrap and prlimit resolve on PATH, and a trivial command
// succeeds end to end through the real pipeline - namespaces,
// resource limits and the seccomp filter together - not just that the
// files exist, which says nothing about whether unprivileged
// namespaces actually work here (a sysadmin can disable them
// system-wide).

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ErrUnavailable reports that this machine cannot run a sandboxed job
// right now: bwrap or prlimit is missing, a binary Available was asked
// about does not exist or is not executable, or the trivial sandboxed
// command Available itself runs failed.
var ErrUnavailable = errors.New("sandbox: unavailable")

// trueBinary is what Available's own smoke test runs: present on
// every Omarchy system as part of coreutils, needs no arguments, and
// exits 0 immediately, so it proves the sandbox mechanism itself works
// without depending on whatever binary the caller actually wants to
// run existing or behaving in any particular way.
const trueBinary = "/usr/bin/true"

// Available reports whether Run can actually sandbox binary on this
// machine: bwrap and prlimit resolve on PATH, binary exists and is
// executable, and a trivial sandboxed command succeeds end to end.
// The expensive part - the trivial run - happens at most once per
// Runner and is cached; binary itself is checked fresh on every call,
// since that is cheap and different calls may name different
// binaries.
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

	result, err := r.Run(ctx, Spec{Binary: trueBinary, OutputDir: dir})
	if err != nil {
		return fmt.Errorf("sandbox: smoke test: %w", err)
	}
	if !result.Succeeded() {
		return fmt.Errorf("%w: smoke test: exit=%d timedOut=%v tooLarge=%v",
			ErrUnavailable, result.ExitCode, result.TimedOut, result.TooLarge)
	}

	return nil
}
