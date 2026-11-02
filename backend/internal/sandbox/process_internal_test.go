package sandbox

// process_internal_test.go tests Run's own start/wait/kill/classify
// logic as an internal test, since it needs to replace the unexported
// command and lookPath fields with fakes: lookPath always succeeds
// without touching this machine's real PATH, and command runs a
// trivial, controllable /bin/sh script instead of the real prlimit
// and bwrap buildArgv would otherwise name - so these tests exercise
// Run's real process lifecycle (including the real Setpgid kill on
// timeout) without depending on bwrap or prlimit being installed at
// all. The real pipeline - prlimit, bwrap and the seccomp filter
// together - is instead covered black-box in sandbox_test.go, skipped
// when this machine cannot actually run it.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// errLookPath stands in for exec.LookPath's own "not found" error in
// TestRun_FailsFastWhenToolsCannotBeResolved, without needing a real
// missing tool to produce one.
var errLookPath = errors.New("test: tool not found")

// fakeRunner returns a Runner whose resolveTools always succeeds
// without touching PATH, and whose command ignores whatever argv
// buildArgv computed and instead runs script through /bin/sh.
func fakeRunner(t *testing.T, script string) *Runner {
	t.Helper()

	return &Runner{
		scratchRoot: t.TempDir(),
		lookPath:    func(string) (string, error) { return "/fake/tool", nil },
		command: func(string, ...string) *exec.Cmd {
			return exec.CommandContext(t.Context(), "/bin/sh", "-c", script)
		},
	}
}

// makeOutputFile creates a regular file of exactly size bytes at
// dir/name, as a sparse file (Truncate, never actually writing size
// bytes of real data): outputReachedSizeLimit only ever looks at the
// size Stat reports, which a sparse file reports correctly, and
// maxOutputFileBytes is large enough that writing it for real in
// every test that needs one would be wasteful.
func makeOutputFile(t *testing.T, dir, name string, size int64) {
	t.Helper()

	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	defer f.Close()

	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncating fixture to %d bytes: %v", size, err)
	}
}

func TestRun_Succeeds(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 0")
	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Succeeded() {
		t.Fatalf("Run() = %+v, want Succeeded() true", result)
	}
}

func TestRun_ReportsNonZeroExitAndClassifiesStderr(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, `echo "Invalid data found when processing input" >&2; exit 1`)
	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded() {
		t.Fatal("Run() reported success for a command that exited 1")
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	if result.StderrClass != classInvalidData {
		t.Errorf("StderrClass = %q, want %q", result.StderrClass, classInvalidData)
	}
}

func TestRun_ReportsTooLargeWhenOutputReachedTheSizeLimit(t *testing.T) {
	t.Parallel()

	// env's own --ignore-signal (see argv.go) makes a real write past
	// prlimit's own --fsize limit fail with EFBIG rather than deliver
	// SIGXFSZ - confirmed directly that the file this leaves behind
	// lands at exactly the limit - so this is what a real too-large
	// job leaves for outputReachedSizeLimit to find: a failing
	// command, and a file already at the cap when it runs.
	// Deliberately not sending a real signal here at all: SIGXFSZ
	// core-dumps by default, and systemd-coredump records every one,
	// with desktop crash notifiers reporting each as a "Process
	// crashed" notification - this is exactly the failure mode this
	// package's own fix avoids, so the test must not reintroduce it
	// to prove the fix works.
	dir := t.TempDir()
	makeOutputFile(t, dir, "out", maxOutputFileBytes)

	r := fakeRunner(t, "exit 1")
	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: dir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TooLarge {
		t.Errorf("TooLarge = false, want true (result: %+v)", result)
	}
	if result.TimedOut {
		t.Error("TimedOut = true, want false")
	}
}

func TestRun_NotTooLargeWhenOutputIsSmallerThanTheLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeOutputFile(t, dir, "out", maxOutputFileBytes-1)

	r := fakeRunner(t, "exit 1")
	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: dir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TooLarge {
		t.Errorf("TooLarge = true, want false for an output one byte under the limit (result: %+v)", result)
	}
}

func TestRun_NotTooLargeWhenTheJobSucceededEvenIfTheOutputIsLarge(t *testing.T) {
	t.Parallel()

	// A large output is only ever evidence of exceeding the limit
	// alongside a job that failed; a successful job's own output
	// reaching this size legitimately is not this package's concern.
	dir := t.TempDir()
	makeOutputFile(t, dir, "out", maxOutputFileBytes)

	r := fakeRunner(t, "exit 0")
	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: dir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TooLarge {
		t.Errorf("TooLarge = true, want false for a successful job (result: %+v)", result)
	}
}

func TestOutputReachedSizeLimit_MissingDirectory(t *testing.T) {
	t.Parallel()

	if outputReachedSizeLimit(filepath.Join(t.TempDir(), "never-created")) {
		t.Error("outputReachedSizeLimit() = true, want false for a directory that does not exist")
	}
}

func TestOutputReachedSizeLimit_IgnoresASubdirectory(t *testing.T) {
	t.Parallel()

	// A subdirectory can never itself be "too large" the way a file
	// can; entry.Info().Mode().IsRegular() must skip it rather than
	// erroring or miscounting it.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatalf("creating fixture subdirectory: %v", err)
	}

	if outputReachedSizeLimit(dir) {
		t.Error("outputReachedSizeLimit() = true, want false for a directory containing only a subdirectory")
	}
}

func TestRun_TimesOutAndKillsTheWholeGroup(t *testing.T) {
	t.Parallel()

	// A child that itself starts a grandchild: both must die once ctx
	// ends, proving the kill reaches the whole process group Setpgid
	// created, not just the single process Run started directly.
	r := fakeRunner(t, "sleep 30 & wait")

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := r.Run(ctx, Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true (result: %+v)", result)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Run() took %s after a 50ms timeout; the sandboxed sleep 30 was not actually killed", elapsed)
	}
}

func TestRun_FailsFastWhenToolsCannotBeResolved(t *testing.T) {
	t.Parallel()

	r := &Runner{
		scratchRoot: t.TempDir(),
		lookPath:    func(string) (string, error) { return "", errLookPath },
	}

	_, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err == nil {
		t.Fatal("Run() error = nil, want an error when tools cannot be resolved")
	}
}

func TestRun_FailsFastOnAnInvalidSpec(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 0")

	_, err := r.Run(t.Context(), Spec{})
	if err == nil {
		t.Fatal("Run() error = nil, want an error for a Spec with no Binary or OutputDir")
	}
}

func TestRun_ReportsStartFailure(t *testing.T) {
	t.Parallel()

	r := &Runner{
		scratchRoot: t.TempDir(),
		lookPath:    func(string) (string, error) { return "/fake/tool", nil },
		command: func(string, ...string) *exec.Cmd {
			return exec.CommandContext(t.Context(), "/no/such/program-for-this-test")
		},
	}

	_, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err == nil {
		t.Fatal("Run() error = nil, want an error when the process cannot even start")
	}
}

func TestRun_ReportsFailureFromAnUnrelatedSignal(t *testing.T) {
	t.Parallel()

	// A signal other than the one Run's own timeout path sends
	// (SIGKILL), so classifyWaitErr's own exit-code extraction still
	// reports -1 for it rather than treating it as success.
	r := fakeRunner(t, "kill -TERM $$; sleep 5")

	result, err := r.Run(t.Context(), Spec{Binary: "/usr/bin/true", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TimedOut || result.TooLarge {
		t.Fatalf("Run() = %+v, want neither TimedOut nor TooLarge for a plain SIGTERM", result)
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 for a signal-killed command", result.ExitCode)
	}
}

func TestClassifyWaitErr_NonExitError(t *testing.T) {
	t.Parallel()

	if exitCode := classifyWaitErr(errLookPath); exitCode != -1 {
		t.Errorf("classifyWaitErr(non-ExitError) = %d, want -1", exitCode)
	}
}

func TestResolveTools_PartialFailure(t *testing.T) {
	t.Parallel()

	// resolveTools looks up env, then bwrap, then prlimit (see argv.go
	// for why env comes first); each of the three must be able to
	// fail on its own, not just the first one tried.
	tests := map[string]struct {
		missing string
	}{
		"env missing":     {missing: "env"},
		"bwrap missing":   {missing: "bwrap"},
		"prlimit missing": {missing: "prlimit"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := &Runner{
				lookPath: func(name string) (string, error) {
					if name == tt.missing {
						return "", errLookPath
					}
					return "/fake/" + name, nil
				},
			}

			if err := r.resolveTools(); err == nil {
				t.Fatalf("resolveTools() error = nil, want an error when %s cannot be resolved", tt.missing)
			}
		})
	}
}

func TestAvailable_SmokeTestFailure(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "exit 1")

	err := r.Available(t.Context(), "/usr/bin/true")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available() error = %v, want %v", err, ErrUnavailable)
	}
}

func TestBoundedStderr_CapsAndTruncates(t *testing.T) {
	t.Parallel()

	w := &boundedStderr{max: 8}

	n, err := w.Write([]byte("1234"))
	if n != 4 || err != nil {
		t.Fatalf("first Write() = (%d, %v), want (4, nil)", n, err)
	}

	// Crosses the cap: only 4 more bytes fit, but the write itself
	// must still be reported as fully accepted.
	n, err = w.Write([]byte("5678ABCD"))
	if n != 8 || err != nil {
		t.Fatalf("second Write() = (%d, %v), want (8, nil)", n, err)
	}

	// Past the cap entirely: nothing more is kept, and this must still
	// never fail the writer.
	n, err = w.Write([]byte("more"))
	if n != 4 || err != nil {
		t.Fatalf("third Write() = (%d, %v), want (4, nil)", n, err)
	}

	if got := string(w.Bytes()); got != "12345678" {
		t.Errorf("Bytes() = %q, want %q", got, "12345678")
	}
}
