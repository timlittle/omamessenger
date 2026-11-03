package sandbox

// process.go starts the argv buildArgv constructs, watches it against
// ctx, and turns however it ended into a Result: run to completion,
// timed out, or killed for writing too much.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Run starts spec.Binary under bwrap, wrapped by prlimit's resource
// limits and this package's own seccomp filter (see argv.go), and
// waits for it to finish or for ctx to end, whichever comes first. A
// non-nil error means the sandbox itself could not be built or
// started; see the package doc comment for why a failed or killed job
// is reported through Result instead.
func (r *Runner) Run(ctx context.Context, spec Spec) (Result, error) {
	if err := spec.validate(); err != nil {
		return Result{}, err
	}
	if err := r.resolveTools(); err != nil {
		return Result{}, err
	}

	seccompFile, err := seccompExtraFile()
	if err != nil {
		return Result{}, fmt.Errorf("sandbox: run: %w", err)
	}
	defer seccompFile.Close() // the child gets its own copy via ExtraFiles when cmd.Start duplicates it; closing ours here never affects that copy

	name, args := buildArgv(spec, r.nicePath, r.ionicePath, r.bwrapPath, r.prlimitPath, r.envPath)

	cmd := r.command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.ExtraFiles = []*os.File{seccompFile}
	cmd.Stdin = nil // os/exec connects a nil Stdin to /dev/null, on top of -nostdin in every transcode profile's own argv

	stderr := &boundedStderr{max: maxCapturedStderr}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("sandbox: run: start: %w", err)
	}

	return wait(ctx, cmd, stderr, spec.Output), nil
}

// wait blocks until cmd exits on its own or ctx ends, killing cmd's
// whole process group in the second case (see killGroup), and turns
// whichever happened into a Result.
func wait(ctx context.Context, cmd *exec.Cmd, stderr *boundedStderr, outputPath string) Result {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		exitCode := classifyWaitErr(waitErr)
		tooLarge := exitCode != 0 && outputReachedSizeLimit(outputPath)

		return Result{ExitCode: exitCode, TooLarge: tooLarge, StderrClass: classifyStderr(stderr.Bytes())}
	case <-ctx.Done():
		killGroup(cmd.Process.Pid)
		<-done // reap; the wait error is moot once this call has deliberately killed the job

		return Result{TimedOut: true, StderrClass: classifyStderr(stderr.Bytes())}
	}
}

// killGroup sends SIGKILL to every process in pid's own group: Run
// always starts a job with Setpgid, so pid is also that group's id.
// bwrap's own --die-with-parent then cascades the kill through its
// internal fork to the sandboxed command itself, even though that
// fork moves to a process group of its own - confirmed directly; see
// docs/decisions.md. A target that has already exited on its own
// between ctx firing and this call is not an error worth
// reporting: the caller is about to reap it anyway.
func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// classifyWaitErr turns cmd.Wait's own error into the job's own exit
// code: 0 for a nil error (success), -1 for a process a signal ended
// (this package's own SIGKILL on timeout, prlimit's own hard CPU
// limit, or the seccomp filter - none of which this package tells
// apart in Result, since no caller needs to), or the process's own
// reported status otherwise. *exec.ExitError's own ExitCode method
// already returns -1 for a signalled process, so this only has its own
// branch for a waitErr that is not an *exec.ExitError at all, which
// Wait never actually returns for a command this package itself
// started and is only waiting on once; that falls back to -1 too,
// rather than being treated as success.
func classifyWaitErr(waitErr error) int {
	if waitErr == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return -1
	}

	return exitErr.ExitCode()
}

// outputReachedSizeLimit reports whether outputPath - Spec.Output's
// own single file - has reached maxOutputFileBytes: the shape
// prlimit's own --fsize limit leaves behind now that env's own
// --ignore-signal (see argv.go) makes a write past it fail with EFBIG
// instead of delivering SIGXFSZ. Confirmed directly that the file this
// leaves lands at exactly the limit, not some smaller,
// write-buffer-dependent size, so no tolerance margin is needed.
// Errors statting it - it never having been written at all is the
// only realistic one, which a job that failed before ffmpeg ever
// opened it would also produce - are treated as "not too large"
// rather than failing the whole classification; so is outputPath
// naming anything but a regular file, which it should never actually
// be.
func outputReachedSizeLimit(outputPath string) bool {
	info, err := os.Stat(outputPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	return info.Size() >= maxOutputFileBytes
}

// boundedStderr collects a job's own stderr up to max bytes,
// discarding anything past that rather than growing without bound:
// Run only ever uses the result to classify a failure into one of
// classify.go's own safe words, never to show or log the raw text, so
// losing the tail of a long, noisy run costs nothing. Write always
// reports success, even once the cap is reached, so a job's own write
// to its stderr never itself fails because this package stopped
// keeping it.
type boundedStderr struct {
	buf bytes.Buffer
	max int
}

// Write appends p to the captured buffer, up to max bytes in total.
func (w *boundedStderr) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		return len(p), nil
	}

	w.buf.Write(p)

	return len(p), nil
}

// Bytes returns everything captured so far.
func (w *boundedStderr) Bytes() []byte {
	return w.buf.Bytes()
}
