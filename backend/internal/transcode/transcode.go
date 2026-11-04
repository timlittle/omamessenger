// Package transcode re-encodes an untrusted video, voice note,
// animation or sticker with the system ffmpeg, run entirely inside a
// sandbox.Runner's bwrap sandbox, so the shell only ever plays a file
// this helper's own fixed ffmpeg invocation produced (see
// docs/decisions.md for each profile's exact arguments).
//
// Each profile (Video, Animation, Audio, Sound, StillFrame) takes an
// input file and an output directory and returns the file it
// produced, or one of ErrUnavailable, ErrTimeout, ErrTooLong or
// ErrFailed, told apart with errors.Is; none ever carries ffmpeg's own
// stderr text.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// runner is what Transcoder needs from a sandbox.Runner: run one job,
// and ask whether the sandbox can run a binary at all.
type runner interface {
	Run(ctx context.Context, spec sandbox.Spec) (sandbox.Result, error)
	Available(ctx context.Context, binary string) error
}

var _ runner = (*sandbox.Runner)(nil)

// Transcoder re-encodes media through ffmpeg, inside a sandbox.
type Transcoder struct {
	runner runner
	ffmpeg string // the absolute path to the system ffmpeg every profile runs

	// slot is a buffered channel of 1: acquire takes the token, release
	// puts it back, so only one job runs at a time (see acquire).
	slot chan struct{}
}

// NewTranscoder returns a Transcoder that runs ffmpegPath - the
// system's own ffmpeg, resolved by the caller - through r.
func NewTranscoder(r runner, ffmpegPath string) *Transcoder {
	return &Transcoder{runner: r, ffmpeg: ffmpegPath, slot: make(chan struct{}, 1)}
}

// acquire reserves this Transcoder's one job slot, waiting until it
// is free or ctx ends first. Every profile shares one slot since
// sandbox's own resource limits are sized for exactly one job at a
// time (see docs/decisions.md). A caller whose ctx ends while waiting
// gets ErrTimeout, the same sentinel a running job's own timeout returns.
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

// profile names one fixed ffmpeg invocation: its argument list, the
// file name it writes, its wall-clock budget, and the largest output
// this package accepts before the sandbox's own backstop would.
type profile struct {
	args       []string
	outputName string
	timeout    time.Duration
	maxOutput  int64

	// rebuild, when set, replaces the sandboxed output checkOutput
	// already approved with a new file this package writes itself
	// from jobDir, reporting its path. The sandboxed output is always
	// removed once rebuild has run, whether it succeeded or not.
	rebuild func(sandboxedOutput, jobDir string) (string, error)
}

// run is every profile method's shared body: reserve a job slot,
// confirm the sandbox can run ffmpeg, give the job a fresh directory,
// run it, and turn the result into the output file's path or one of
// this package's sentinel errors. jobDir is removed on every failure
// path; only a successful return may leave it behind (see
// app.transcodedPlayCopy's own cleanup).
func (t *Transcoder) run(ctx context.Context, in, outDir string, p profile) (path string, err error) {
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
	defer func() {
		if err != nil {
			_ = os.RemoveAll(jobDir) // best-effort: run is already returning the real error
		}
	}()

	jobInput, err := placeJobInput(in, jobDir)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = os.Remove(jobInput) // best-effort: nothing past this point, success or failure, still needs a copy of the caller's own input file
	}()

	outputPath, err := createJobOutput(jobDir, p.outputName)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	spec := sandbox.Spec{Binary: t.ffmpeg, Args: p.args, Input: jobInput, Output: outputPath}

	result, err := t.runner.Run(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("transcode: %w", err)
	}
	if err := resultError(result); err != nil {
		return "", err
	}
	if err := checkOutput(outputPath, p.maxOutput); err != nil {
		return "", err
	}
	return finishOutput(p, outputPath, jobDir)
}

// finishOutput returns outputPath unchanged when p has no rebuild
// step, or replaces it with p.rebuild's file otherwise, always
// removing the sandboxed file at outputPath afterward.
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

// checkOutput confirms path exists, is a regular file (Lstat, not
// Stat, so a symlink left by a misbehaving ffmpeg is caught rather
// than followed), and is not empty and no larger than max.
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
