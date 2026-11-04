// Package sandbox runs one external command at a time inside a
// bubblewrap (bwrap) sandbox, wrapped by prlimit's resource limits and
// this package's own seccomp filter, so a crafted media file reaches
// ffmpeg's C decoders only inside a process with almost no view of
// this machine, no network, and a bounded run time (see
// docs/decisions.md for the exact flags).
//
// Spec names one job: a binary, its arguments, an optional input file
// bound read-only at InputPath, and one output file bound read-write
// under OutputDirPath. Every path and flag in a Spec must be a
// literal the caller chose, never a string controlled by a sender or
// the file being processed, since argv is readable by any local user
// through /proc.
//
// Run binds only the one file named by Output, never a whole
// directory, so a job can never reach anything else in the caller's
// job directory.
//
// Run's own error return covers only the sandbox itself failing to
// start or run; once a job has started, Result says how it fared,
// since a sandboxed ffmpeg exiting non-zero on a hostile file is an
// ordinary outcome, not an infrastructure failure.
package sandbox

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
)

// Spec describes one command to run inside the sandbox.
type Spec struct {
	// Binary is the absolute, literal path to the program bwrap execs
	// once the sandbox is set up.
	Binary string

	// Args are Binary's own arguments, in order. Like Binary, every
	// element must already be a literal the caller chose, never a
	// string read from the file being processed.
	Args []string

	// Input, when set, is bound read-only inside the sandbox at
	// InputPath. Its base name must be InputBaseName; validate
	// refuses anything else.
	Input string

	// Output is the one file Run binds read-write inside the sandbox,
	// under OutputDirPath, at its own base name. The caller creates
	// this file itself, empty and 0600, before calling Run; Run never
	// binds anything else under OutputDirPath.
	Output string
}

// validate reports an error unless s has everything Run needs: Binary,
// Output, and, if given, an Input named InputBaseName.
func (s Spec) validate() error {
	if s.Binary == "" {
		return fmt.Errorf("sandbox: spec: binary is required")
	}
	if s.Binary[0] != '/' {
		return fmt.Errorf("sandbox: spec: binary must be an absolute path")
	}
	if s.Output == "" {
		return fmt.Errorf("sandbox: spec: output file is required")
	}
	if s.Input != "" && filepath.Base(s.Input) != InputBaseName {
		return fmt.Errorf("sandbox: spec: input must be named %q", InputBaseName)
	}
	return nil
}

// Result is one Run call's outcome, once its job actually started.
type Result struct {
	// ExitCode is the command's exit status, meaningful only when
	// neither TimedOut nor TooLarge is true; -1 when killed by a
	// signal this package has no named field for (see classifyWaitErr).
	ExitCode int

	// TimedOut is true when ctx's deadline ended the job first; Run
	// always kills the job's whole process group in this case.
	TimedOut bool

	// TooLarge is true when the job failed and left behind a file
	// that reached maxOutputFileBytes: prlimit's file-size limit turns
	// an over-limit write into EFBIG rather than a kill, so this is
	// inferred from the file left behind.
	TooLarge bool

	// StderrClass is a short, fixed, safe word summarising the job's
	// captured stderr (see classify.go), so a failure can be logged
	// without logging raw, possibly sender-controlled text.
	StderrClass string
}

// Succeeded reports whether the command ran to completion with exit
// status 0, neither timed out nor killed for writing too much.
func (r Result) Succeeded() bool {
	return !r.TimedOut && !r.TooLarge && r.ExitCode == 0
}

// Runner runs sandboxed jobs, one at a time per call, on one machine.
// The zero value is not usable; construct one with NewRunner.
type Runner struct {
	// scratchRoot is where Available's smoke test creates and removes
	// a throwaway job directory; it must already exist. Never used for
	// a real job's own Output.
	scratchRoot string

	toolsOnce   sync.Once
	envPath     string
	bwrapPath   string
	prlimitPath string
	nicePath    string
	ionicePath  string
	toolsErr    error

	smokeOnce sync.Once
	smokeErr  error

	// command builds the process Run starts. Real jobs use
	// exec.Command; tests replace it with a stub so Run's own
	// start/wait/kill/classify logic can be exercised without a real
	// bwrap.
	command func(name string, args ...string) *exec.Cmd

	// lookPath resolves a tool's name to its absolute path. Real jobs
	// use exec.LookPath; tests replace it so resolveTools can succeed
	// or fail independent of what tools this machine actually has.
	lookPath func(file string) (string, error)
}

// NewRunner returns a Runner whose Available method creates and
// removes throwaway directories under scratchRoot, which must already
// exist.
func NewRunner(scratchRoot string) *Runner {
	return &Runner{scratchRoot: scratchRoot, command: exec.Command, lookPath: exec.LookPath}
}

// resolveTools finds nice, ionice, env, bwrap and prlimit once and
// caches the result, in the same order buildArgv chains them in, so
// the first missing tool is also the first one reported.
func (r *Runner) resolveTools() error {
	r.toolsOnce.Do(func() {
		nicePath, err := r.lookPath("nice")
		if err != nil {
			r.toolsErr = fmt.Errorf("%w: nice: %w", ErrUnavailable, err)
			return
		}

		ionicePath, err := r.lookPath("ionice")
		if err != nil {
			r.toolsErr = fmt.Errorf("%w: ionice: %w", ErrUnavailable, err)
			return
		}

		envPath, err := r.lookPath("env")
		if err != nil {
			r.toolsErr = fmt.Errorf("%w: env: %w", ErrUnavailable, err)
			return
		}

		bwrapPath, err := r.lookPath("bwrap")
		if err != nil {
			r.toolsErr = fmt.Errorf("%w: bwrap: %w", ErrUnavailable, err)
			return
		}

		prlimitPath, err := r.lookPath("prlimit")
		if err != nil {
			r.toolsErr = fmt.Errorf("%w: prlimit: %w", ErrUnavailable, err)
			return
		}

		r.nicePath, r.ionicePath, r.envPath, r.bwrapPath, r.prlimitPath = nicePath, ionicePath, envPath, bwrapPath, prlimitPath
	})

	return r.toolsErr
}
