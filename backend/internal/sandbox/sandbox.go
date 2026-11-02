// Package sandbox runs one external command at a time inside a
// bubblewrap (bwrap) sandbox, wrapped by prlimit's own resource limits
// and this package's own seccomp syscall filter: the "Rule of 2"
// pattern (never let an untrustworthy input reach an unsafe-language
// parser at full privilege) applied to this helper's own use of the
// system ffmpeg, so a crafted media file reaches ffmpeg's C decoders
// only inside a process that can see almost nothing of this machine,
// reach no network, and run for only so long. See docs/decisions.md
// for the exact bwrap and prlimit flags this package runs, why each
// one is there, and what the seccomp filter denies.
//
// Spec names one job: a binary, its own arguments, an optional input
// file bound read-only at InputPath, and one output file bound
// read-write under OutputDirPath. Every path and flag in a Spec must
// be a literal the caller's own code chose - never a string a sender,
// another service, or the file being processed controls - since argv
// is readable by any other local user through /proc the same way a
// command's name already is (see .golangci.yml's forbidigo rule); the
// fixed, sandbox-internal paths InputPath and OutputDirPath exist so a
// caller never has to put a real, sender-influenced path into argv at
// all.
//
// Run binds only the one file named by Output read-write - never a
// whole directory - so a job can never reach anything else a caller's
// own job directory happens to also hold, such as Input's own hard
// link (see Output's own doc comment and docs/decisions.md).
//
// Run's own error return is only ever about the sandbox itself failing
// to start or run at all (bwrap or prlimit missing, the process could
// not be started, ctx already done). Once a job has actually started,
// how it fared - succeeded, exited with a failure, ran out of time, or
// wrote more than its output is allowed to - is Result's job to say.
// This split matters because a sandboxed ffmpeg exiting non-zero on a
// corrupt or hostile file is an entirely ordinary outcome this
// package's own callers must handle every time, not an infrastructure
// failure.
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
	// element must already be a literal this caller's own code chose -
	// a fixed flag, a number, or InputPath/OutputDirPath - never a
	// string read from the file being processed.
	Args []string

	// Input, when set, is bound read-only inside the sandbox at
	// InputPath. Its own base name must be InputBaseName - validate
	// refuses anything else - since this literal path, not InputPath,
	// is what actually reaches bwrap's own argv (see the package doc
	// comment and InputBaseName's own).
	Input string

	// Output is the one file Run binds read-write inside the sandbox,
	// under OutputDirPath, at that file's own base name: the only
	// place a job may write to. The caller creates this file itself -
	// empty, 0600 - before calling Run (transcode's own
	// joboutput.go does exactly that for every profile); Run never
	// creates it and never binds anything else under OutputDirPath, so
	// a job can address nothing there but this one file (see
	// docs/decisions.md).
	Output string
}

// validate reports an error unless s has everything Run needs to build
// a sandbox around it: Binary and Output are the two fields every job
// needs regardless of what it does once it starts, and Input, when
// given at all, must already be named InputBaseName - the one check
// standing between a caller's mistake and a sender- or user-chosen
// path reaching bwrap's own argv (see Spec's own doc comment).
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
	// ExitCode is the command's own exit status: meaningful only when
	// neither TimedOut nor TooLarge is true. -1 when the command was
	// killed by a signal this package does not give its own named
	// field to (see classifyWaitErr) - the seccomp filter's own
	// ActionKillProcess response among them.
	ExitCode int

	// TimedOut is true when ctx's own deadline ended the job before it
	// finished on its own; Run always kills the job's whole process
	// group in this case (see wait).
	TimedOut bool

	// TooLarge is true when the job failed and left behind a file
	// that reached maxOutputFileBytes (see limits.go and
	// outputReachedSizeLimit): prlimit's own file-size limit turned a
	// write past it into an ordinary EFBIG error rather than a kill,
	// so this is inferred from what the job left behind, not from how
	// it died.
	TooLarge bool

	// StderrClass is a short, fixed, safe word summarising what the
	// job's own captured stderr looked like (see classify.go), for
	// logging a failure's shape without ever logging the raw text,
	// which could echo a sender-controlled file name or path.
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
	// scratchRoot is where Available's own smoke test creates and
	// removes a throwaway job directory and output file; it must
	// already exist. Never used for a real job's own Output, which
	// every caller of Run supplies itself.
	scratchRoot string

	toolsOnce   sync.Once
	envPath     string
	bwrapPath   string
	prlimitPath string
	toolsErr    error

	smokeOnce sync.Once
	smokeErr  error

	// command builds the process Run starts: bwrap and prlimit's own
	// resolved paths never change once resolveTools has run, so this
	// takes only the name and args buildArgv already computed. Real
	// jobs use exec.Command; tests replace it with a stub that runs a
	// trivial, controllable program instead, so Run's own start/wait/
	// kill/classify logic can be exercised without a real bwrap.
	command func(name string, args ...string) *exec.Cmd

	// lookPath resolves a tool's name to its absolute path. Real jobs
	// use exec.LookPath; tests replace it so resolveTools can be made
	// to succeed (or fail) without depending on whether this machine
	// actually has bwrap and prlimit installed, which command's own
	// fake already makes irrelevant to what actually runs.
	lookPath func(file string) (string, error)
}

// NewRunner returns a Runner whose Available method creates and
// removes its own throwaway directories under scratchRoot, which must
// already exist. It does no I/O itself.
func NewRunner(scratchRoot string) *Runner {
	return &Runner{scratchRoot: scratchRoot, command: exec.Command, lookPath: exec.LookPath}
}

// resolveTools finds env, bwrap and prlimit (see lookPath) once,
// caching the result for every later call on this Runner: a PATH that
// does not change while this process runs, which holds for the
// helper's own fixed install layout.
func (r *Runner) resolveTools() error {
	r.toolsOnce.Do(func() {
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

		r.envPath, r.bwrapPath, r.prlimitPath = envPath, bwrapPath, prlimitPath
	})

	return r.toolsErr
}
