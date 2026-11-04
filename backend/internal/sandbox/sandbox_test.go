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

	result, err := r.Run(ctx, sandbox.Spec{Binary: "/usr/bin/true", Output: mustOutputFile(t)})
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
		Binary: "/usr/bin/bash",
		Args:   []string{"-c", "exec 3<>/dev/tcp/1.1.1.1/80"},
		Output: mustOutputFile(t),
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
		Binary: "/usr/bin/cat",
		Args:   []string{outside},
		Output: mustOutputFile(t),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded() {
		t.Fatal("Run() read a host file outside Input/Output; it should not have been visible inside the sandbox at all")
	}
}

func TestRun_SeccompDeniesUnshare(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/unshare")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spec := sandbox.Spec{
		Binary: "/usr/bin/unshare",
		Args:   []string{"-U", "/usr/bin/true"},
		Output: mustOutputFile(t),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// The filter refuses a denied syscall with EPERM rather than
	// killing the process (see seccomp.go's own package doc comment
	// for why): unshare(2) itself just fails, and the unshare binary
	// reports that failure through its own, ordinary non-zero exit -
	// never a signal, so this checks that the job failed at all,
	// without pinning the exact code to one userspace tool's own
	// convention for reporting a failed syscall.
	if result.Succeeded() {
		t.Fatal("Run() succeeded calling unshare; the seccomp filter did not deny it")
	}
	if result.TimedOut || result.TooLarge {
		t.Fatalf("Run() = %+v, want an ordinary failure, not a timeout or an oversized output", result)
	}
}

func TestRun_SeccompDeniesForkingAChildProcess(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/bash")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// "cmd & wait" needs a child process - by definition, not a
	// thread - to run cmd in the background: exactly the shape of call
	// this package's own seccomp filter now refuses outright (see
	// seccomp.go's own processCreationSyscalls), whichever of fork,
	// vfork or a thread-less clone bash actually uses for it.
	spec := sandbox.Spec{
		Binary: "/usr/bin/bash",
		Args:   []string{"-c", "true & wait"},
		Output: mustOutputFile(t),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded() {
		t.Fatal("Run() succeeded forking a child process; the seccomp filter did not deny it")
	}
	if result.TimedOut || result.TooLarge {
		t.Fatalf("Run() = %+v, want an ordinary failure, not a timeout or an oversized output", result)
	}
}

func TestRun_OutputBindExposesOnlyTheSingleBoundFile(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/bash")

	// input and output live in the same host directory, the same way
	// transcode's own job directory holds both the hard-linked input
	// and the pre-created output file side by side - exactly the
	// layout that used to let a job reach the hard link for writing
	// through the old, whole-directory output bind (see
	// docs/decisions.md).
	jobDir := t.TempDir()
	inputPath := filepath.Join(jobDir, sandbox.InputBaseName)
	if err := os.WriteFile(inputPath, []byte("original input bytes"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	outputPath := filepath.Join(jobDir, "out.bin")
	if err := os.WriteFile(outputPath, nil, 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Every piece here is a bash builtin (redirection, :, [, a plain
	// variable) rather than an external command: TestRun_
	// SeccompDeniesForkingAChildProcess already proves this package's
	// own seccomp filter now refuses the fork a real external command
	// like touch or rm would need, so using one here would make this
	// test confound that unrelated protection with the one this test
	// actually checks - whether anything other than the bound file is
	// writable at all.
	script := `echo ffmpeg output > /work/out/out.bin; wrote=$?
: > /work/out/newfile 2>/dev/null
[ ! -e /work/out/newfile ] && [ "$wrote" = 0 ]`

	spec := sandbox.Spec{
		Binary: "/usr/bin/bash",
		Args:   []string{"-c", script},
		Input:  inputPath,
		Output: outputPath,
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Succeeded() {
		t.Fatalf("Run() = %+v, want Succeeded() true: writing the bound file must still work, and "+
			"creating any other file under OutputDirPath must fail", result)
	}

	got, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("reading input fixture back: %v", err)
	}
	if string(got) != "original input bytes" {
		t.Fatalf("the host's own input file changed to %q; the sandbox could reach it through the output bind", got)
	}
}

func TestRun_SeccompDeniesSystemVSharedMemory(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/python3")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// python3's own ctypes module calls straight into libc's shmget,
	// the exact route a hijacked ffmpeg would use to allocate memory
	// the kernel charges to no process's own RLIMIT_AS at all (see
	// ipcSyscalls' own doc comment in seccomp.go): a one-megabyte
	// segment, IPC_CREAT so it does not need an existing key, read-
	// write for the owner only. shmget returns -1 on any failure, so
	// this does not depend on EPERM's own numeric value, only on the
	// call failing at all.
	script := `
import ctypes
libc = ctypes.CDLL(None, use_errno=True)
result = libc.shmget(ctypes.c_long(0), ctypes.c_size_t(1024 * 1024), 0o600 | 0o1000)
raise SystemExit(0 if result == -1 else 1)
`

	spec := sandbox.Spec{
		Binary: "/usr/bin/python3",
		Args:   []string{"-c", script},
		Output: mustOutputFile(t),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Succeeded() {
		t.Fatalf("Run() = %+v, want Succeeded() true: the script itself reports success only when shmget failed", result)
	}
}

func TestRun_RootDevAndOversizedDevShmWritesAllFail(t *testing.T) {
	t.Parallel()

	r := requireSandbox(t, "/usr/bin/bash")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Every place inside the sandbox a hijacked job might still try to
	// write, now that bwrap's own root and /dev are both remounted
	// read-only and /dev/shm has a cap of its own (see
	// docs/decisions.md): the root tmpfs bwrap creates by default,
	// /dev itself (outside the small, separate tmpfs now capped at
	// /dev/shm), and /work outside the one file actually bound there,
	// must all fail. /dev/shm within its own cap must still succeed -
	// this is not meant to make it read-only too, only bounded - so a
	// fix that overshoots by refusing everything under /dev/shm would
	// wrongly pass the rest of this test without that line failing it.
	// Every step is a bash builtin (printf, redirection, [): an
	// external command would need the fork this package's seccomp
	// filter now refuses outright (see
	// TestRun_SeccompDeniesForkingAChildProcess), which would confound
	// that unrelated protection with the one this test actually
	// checks.
	script := `
fail=0
printf probe > /newfile 2>/dev/null && fail=1
printf probe > /dev/newfile 2>/dev/null && fail=1
printf probe > /work/newfile 2>/dev/null && fail=1
printf probe > /dev/shm/ok 2>/dev/null || fail=1
printf "%*s" 16000000 "" > /dev/shm/big1 2>/dev/null
printf "%*s" 1000000 "" > /dev/shm/big2 2>/dev/null && fail=1
[ "$fail" = 0 ]
`

	spec := sandbox.Spec{
		Binary: "/usr/bin/bash",
		Args:   []string{"-c", script},
		Output: mustOutputFile(t),
	}

	result, err := r.Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Succeeded() {
		t.Fatalf("Run() = %+v, want Succeeded() true: every write outside the sandbox's own bounded places should have failed, and the one within /dev/shm's own cap should have succeeded", result)
	}
}
