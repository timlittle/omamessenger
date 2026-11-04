package sandbox

// process.go starts the sandboxed command and turns how it ended into a Result.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Run runs spec.Binary in the sandbox until it exits or ctx ends. An error
// means the sandbox could not start; a failed or killed job is in Result.
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
	defer seccompFile.Close() // cmd.Start duplicates it into the child via ExtraFiles

	name, args := buildArgv(spec, toolPaths{
		nice: r.nicePath, ionice: r.ionicePath, env: r.envPath, prlimit: r.prlimitPath, bwrap: r.bwrapPath,
	})

	cmd := r.command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.ExtraFiles = []*os.File{seccompFile}
	cmd.Stdin = nil // os/exec connects a nil Stdin to /dev/null

	stderr := &boundedStderr{max: maxCapturedStderr}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("sandbox: run: start: %w", err)
	}

	return wait(ctx, cmd, stderr, spec.Output), nil
}

// wait blocks until cmd exits or ctx ends, killing it in the second case.
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
		<-done // reap; its error no longer matters once we've killed the job

		return Result{TimedOut: true, StderrClass: classifyStderr(stderr.Bytes())}
	}
}

// killGroup sends SIGKILL to pid's process group; bwrap's --die-with-parent
// passes it on to the sandboxed command (see docs/decisions.md).
func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL) // a group that already exited needs no report
}

// classifyWaitErr turns cmd.Wait's error into an exit code, -1 when signalled.
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

// outputReachedSizeLimit reports whether outputPath hit prlimit's file size limit.
func outputReachedSizeLimit(outputPath string) bool {
	info, err := os.Stat(outputPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	return info.Size() >= maxOutputFileBytes
}

// boundedStderr keeps the first max bytes of a job's stderr, enough to
// classify a failure; the text is never shown or logged.
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
