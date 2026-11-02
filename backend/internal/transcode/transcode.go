// Package transcode re-encodes an untrusted video, voice note,
// animation or sticker with the system ffmpeg, run entirely inside a
// sandbox.Runner's own bwrap sandbox, so the shell only ever plays a
// file this helper's own, fixed ffmpeg invocation produced - never
// bytes a sender's file shaped directly (the "Rule of 2": never let
// an untrustworthy input reach an unsafe-language parser at full
// privilege). See docs/decisions.md for each profile's exact ffmpeg
// arguments and why they are there.
//
// Each profile - Video, Animation, Audio, Sound, StillFrame - takes
// an input file and an output directory and returns the one file it
// produced there, or one of this package's own sentinel errors:
// ErrUnavailable (the sandbox cannot run right now), ErrTimeout (the
// job's own wall-clock budget ran out), ErrTooLong (the output grew
// past what this profile allows) or ErrFailed (anything else - a
// corrupt file, an unsupported stream, ffmpeg itself failing). A
// caller tells them apart with errors.Is; none of the four ever
// carries ffmpeg's own stderr text, which this package never exposes.
//
// By the time Run returns, the sandboxed ffmpeg is not merely
// finished but gone: bwrap gives it its own PID namespace, and a PID
// namespace is torn down - every process in it killed - the moment
// its own pid 1 (bwrap's own internal fork, see sandbox's own package
// doc comment) exits, which is what Run's own wait is blocked on.
// checkOutput's own Lstat check, run after Run has already returned,
// is therefore never racing a still-live process that could still
// change what it finds.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// ErrUnavailable reports that the sandbox cannot run a job right now
// (see sandbox.Runner.Available).
var ErrUnavailable = errors.New("transcode: sandbox unavailable")

// ErrTimeout reports that a job's own wall-clock budget ran out
// before ffmpeg finished.
var ErrTimeout = errors.New("transcode: timed out")

// ErrTooLong reports that a job's own output grew past the limit this
// profile allows, whether prlimit's own backstop inside the sandbox or
// this package's own, tighter check afterwards caught it.
var ErrTooLong = errors.New("transcode: output exceeded its size limit")

// ErrFailed reports any other failure: a corrupt or unsupported file,
// or ffmpeg itself failing for a reason this package gives no other
// sentinel to.
var ErrFailed = errors.New("transcode: failed")

// runner is what Transcoder needs from a sandbox.Runner: enough to
// run one job and ask whether the sandbox can run a binary at all.
// Defined here, at the consumer, per this project's own interface
// rules (see .claude/rules/interfaces.md); *sandbox.Runner is
// Transcoder's only real implementation, a test fake its second.
type runner interface {
	Run(ctx context.Context, spec sandbox.Spec) (sandbox.Result, error)
	Available(ctx context.Context, binary string) error
}

var _ runner = (*sandbox.Runner)(nil)

// Transcoder re-encodes media through ffmpeg, inside a sandbox.
type Transcoder struct {
	runner runner
	ffmpeg string // the absolute path to the system ffmpeg every profile runs

	// slot is a buffered channel of 1, holding one token: acquire
	// takes it, release puts it back, so only one job runs through
	// this Transcoder's own runner at a time. See acquire's own doc
	// comment for why.
	slot chan struct{}
}

// NewTranscoder returns a Transcoder that runs ffmpegPath - the
// system's own ffmpeg, resolved by the caller - through r.
func NewTranscoder(r runner, ffmpegPath string) *Transcoder {
	return &Transcoder{runner: r, ffmpeg: ffmpegPath, slot: make(chan struct{}, 1)}
}

// acquire reserves this Transcoder's one job slot, waiting until it
// is free or ctx ends first. Every profile shares one slot because a
// single job already runs ffmpeg at full tilt on its own two threads,
// and because sandbox's own resource limits (see docs/decisions.md)
// are sized for exactly one job at a time, not several sharing them.
//
// A caller whose ctx ends while still waiting gets ErrTimeout: the
// same sentinel a job that ran out of time once it actually started
// already returns, since both mean the same thing to a caller - this
// call did not finish within the time it was given - and a caller
// checking with errors.Is need not tell the two apart.
func (t *Transcoder) acquire(ctx context.Context) error {
	select {
	case t.slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: waiting for a transcode slot: %w", ErrTimeout, ctx.Err())
	}
}

// release frees the job slot acquire reserved.
func (t *Transcoder) release() {
	<-t.slot
}

// profile names one fixed ffmpeg invocation this package knows how to
// run: its own argument list, the file name it writes inside
// sandbox.OutputDirPath, the wall-clock budget the job gets, and the
// largest output this package itself accepts before even the
// sandbox's own, blunter backstop would.
type profile struct {
	args       []string
	outputName string
	timeout    time.Duration
	maxOutput  int64

	// rebuild, when set, replaces the sandboxed output checkOutput
	// already approved with a new file this package writes itself
	// from jobDir, and reports that file's own path. The sandboxed
	// output named by sandboxedOutput is always removed once rebuild
	// has run, whether it succeeded or not: nothing has any further
	// use for it either way.
	rebuild func(sandboxedOutput, jobDir string) (string, error)
}

// run is every profile method's own shared body: reserve this
// Transcoder's one job slot, confirm the sandbox can run ffmpeg at
// all, give the job a fresh directory under outDir and its own
// wall-clock budget, run it, and turn whatever happened into the one
// output file's path or one of this package's own sentinel errors.
func (t *Transcoder) run(ctx context.Context, in, outDir string, p profile) (string, error) {
	if err := t.acquire(ctx); err != nil {
		return "", err
	}
	defer t.release()

	if err := t.runner.Available(ctx, t.ffmpeg); err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	jobDir, err := os.MkdirTemp(outDir, "transcode-*")
	if err != nil {
		return "", fmt.Errorf("transcode: job directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	spec := sandbox.Spec{Binary: t.ffmpeg, Args: p.args, Input: in, OutputDir: jobDir}

	result, err := t.runner.Run(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("transcode: %w", err)
	}
	if err := resultError(result); err != nil {
		return "", err
	}

	outputPath := filepath.Join(jobDir, p.outputName)
	if err := checkOutput(outputPath, p.maxOutput); err != nil {
		return "", err
	}
	return finishOutput(p, outputPath, jobDir)
}

// finishOutput returns outputPath unchanged when p has no rebuild
// step, or replaces it with p.rebuild's own file otherwise. The
// sandboxed file at outputPath is always removed once rebuild has
// run, whether it succeeded or not.
func finishOutput(p profile, outputPath, jobDir string) (string, error) {
	if p.rebuild == nil {
		return outputPath, nil
	}

	finalPath, rebuildErr := p.rebuild(outputPath, jobDir)
	switch removeErr := os.Remove(outputPath); {
	case rebuildErr != nil:
		return "", rebuildErr
	case removeErr != nil:
		return "", fmt.Errorf("transcode: removing intermediate: %w", removeErr)
	default:
		return finalPath, nil
	}
}

// resultError turns a sandbox.Result into one of this package's own
// sentinel errors, or nil for an ordinary successful exit.
func resultError(result sandbox.Result) error {
	switch {
	case result.TimedOut:
		return ErrTimeout
	case result.TooLarge:
		return ErrTooLong
	case !result.Succeeded():
		return fmt.Errorf("%w: ffmpeg exited %d", ErrFailed, result.ExitCode)
	default:
		return nil
	}
}

// checkOutput confirms path exists, is a regular file - never a
// symlink - and is not empty and no larger than max: ffmpeg reporting
// success is not, on its own, proof it actually wrote a usable file.
//
// Lstat, not Stat: a compromised or merely misbehaving ffmpeg could
// leave path as a symlink to any file on the host instead of writing
// one itself, and whatever later opens, copies or serves "the
// transcoded output" would otherwise follow it there. This is not a
// race to close, only a lie to catch: the sandboxed job is a dead PID
// namespace by the time Run returns (see package doc comment), so
// there is no process left that could still be swapping the target
// between this check and whatever reads the file next.
func checkOutput(path string, max int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: output missing: %w", ErrFailed, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: output is not a regular file", ErrFailed)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%w: output is empty", ErrFailed)
	}
	if info.Size() > max {
		return fmt.Errorf("%w: output is %d bytes", ErrTooLong, info.Size())
	}

	return nil
}
