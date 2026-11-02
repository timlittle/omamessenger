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

	name, args := buildArgv(spec, r.bwrapPath, r.prlimitPath)

	cmd := r.command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.ExtraFiles = []*os.File{seccompFile}
	cmd.Stdin = nil // os/exec connects a nil Stdin to /dev/null, on top of -nostdin in every transcode profile's own argv

	stderr := &boundedStderr{max: maxCapturedStderr}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("sandbox: run: start: %w", err)
	}

	return wait(ctx, cmd, stderr), nil
}

// wait blocks until cmd exits on its own or ctx ends, killing cmd's
// whole process group in the second case (see killGroup), and turns
// whichever happened into a Result.
func wait(ctx context.Context, cmd *exec.Cmd, stderr *boundedStderr) Result {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		exitCode, tooLarge := classifyWaitErr(waitErr)
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
// fork moves to a process group of its own - confirmed on this
// machine; see docs/decisions.md. A target that has already exited on
// its own between ctx firing and this call is not an error worth
// reporting: the caller is about to reap it anyway.
func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// classifyWaitErr turns cmd.Wait's own error into an exit code and
// whether prlimit's own output-size limit (SIGXFSZ) was what ended
// the job, rather than an ordinary non-zero exit or another signal
// (the seccomp filter's own ActionKillProcess response among them,
// which this package does not give its own named Result field, since
// no caller needs to tell it apart from any other failure). A nil err
// means exit status 0; any error that is not an *exec.ExitError -
// which Wait never actually returns for a command this package itself
// started and is only waiting on once - falls back to exit code -1
// rather than being treated as success.
func classifyWaitErr(waitErr error) (exitCode int, tooLarge bool) {
	if waitErr == nil {
		return 0, false
	}

	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return -1, false
	}

	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return -1, status.Signal() == syscall.SIGXFSZ
	}

	return exitErr.ExitCode(), false
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
