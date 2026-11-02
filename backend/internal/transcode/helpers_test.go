package transcode_test

// helpers_test.go holds the test doubles every test file in this
// package needs: fakeRunner, a hand-written fake satisfying the
// small, unexported interface Transcoder.run actually calls, so a
// profile's own wiring - which output name, timeout and size cap it
// uses - and its own fixed ffmpeg argument list can both be checked
// through the public API alone, by inspecting what reached Run,
// rather than needing access to anything unexported; and
// blockingRunner, which holds Run open until a test lets it finish,
// for proving two jobs on the same Transcoder never run at once.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

// fakeRunner stands in for a *sandbox.Runner: Available and Run both
// return whatever a test configured, and Run also writes
// writeOutputName under the job's own OutputDir when set, simulating
// what a real ffmpeg run inside the sandbox would have left there.
type fakeRunner struct {
	availErr error

	result sandbox.Result
	runErr error

	writeOutputName string
	writeOutputData []byte

	// writeOutputSymlinkTo, when set instead of writeOutputData, makes
	// Run create writeOutputName as a symlink to this target rather
	// than an ordinary file, simulating a compromised or misbehaving
	// ffmpeg leaving a symlink where the sandbox told it to write its
	// real output.
	writeOutputSymlinkTo string

	// lastSpec is the Spec the most recent Run call received, for a
	// test to check a profile's own argv and paths against.
	lastSpec sandbox.Spec
}

// Available returns availErr, whatever a test set it to.
func (f *fakeRunner) Available(_ context.Context, _ string) error {
	return f.availErr
}

// Run records spec, optionally writes a fake output file where the
// caller's own OutputDir says to, and returns result and runErr.
func (f *fakeRunner) Run(_ context.Context, spec sandbox.Spec) (sandbox.Result, error) {
	f.lastSpec = spec

	if f.runErr != nil {
		return sandbox.Result{}, f.runErr
	}

	if f.writeOutputName != "" {
		path := filepath.Join(spec.OutputDir, f.writeOutputName)

		if f.writeOutputSymlinkTo != "" {
			if err := os.Symlink(f.writeOutputSymlinkTo, path); err != nil {
				return sandbox.Result{}, err
			}
		} else {
			data := f.writeOutputData
			if data == nil {
				data = []byte("fake ffmpeg output")
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return sandbox.Result{}, err
			}
		}
	}

	return f.result, nil
}

// blockingRunner is a runner whose Run does not return until the test
// sends on unblock, for proving that Transcoder lets only one job run
// through it at a time. Available always succeeds immediately: only
// Run's own duration matters to that test. Safe to call Run on more
// than once (started only ever closes the first time), so the same
// instance can also be used to check that a slot it held is free
// again once unblock has already been closed.
type blockingRunner struct {
	startedOnce sync.Once
	// started is closed the first time Run is actually called, so a
	// test can wait for that rather than racing it with a sleep.
	started chan struct{}
	// unblock is read by Run; sending on it (or closing it) lets any
	// call currently blocked in Run return result.
	unblock chan struct{}
	result  sandbox.Result
}

// newBlockingRunner returns a blockingRunner with its own channels
// already made, ready to use.
func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}), unblock: make(chan struct{})}
}

// Available always succeeds.
func (b *blockingRunner) Available(context.Context, string) error { return nil }

// Run marks itself started, then blocks until unblock is sent to or
// closed, or ctx ends first.
func (b *blockingRunner) Run(ctx context.Context, _ sandbox.Spec) (sandbox.Result, error) {
	b.startedOnce.Do(func() { close(b.started) })

	select {
	case <-b.unblock:
		return b.result, nil
	case <-ctx.Done():
		return sandbox.Result{}, ctx.Err()
	}
}

// requireRealTranscoder returns a Transcoder backed by a real
// sandbox.Runner and the host's own ffmpeg, skipping the test unless
// this machine can actually run a job end to end right now.
func requireRealTranscoder(t *testing.T) *transcode.Transcoder {
	t.Helper()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH")
	}

	r := sandbox.NewRunner(t.TempDir())
	if err := r.Available(t.Context(), ffmpegPath); err != nil {
		t.Skipf("sandbox unavailable on this machine: %v", err)
	}

	return transcode.NewTranscoder(r, ffmpegPath)
}

// integrationDir returns a fresh directory under the repository's own
// build/ (never /tmp, see resources.md), three levels above this
// package, for an integration test's own fixtures and transcode
// output; it is removed once the test ends, pass or fail.
func integrationDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("../../../build", "transcode-it-*")
	if err != nil {
		t.Fatalf("creating build/ scratch directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // best-effort: a throwaway directory this test alone created

	return dir
}

// generateFixture runs the host's own ffmpeg - never this package's
// sandboxed one, which only ever runs through Transcoder - to build a
// small synthetic input file at filepath.Join(dir, name) from args,
// skipping the test if ffmpeg cannot produce it: an optional encoder,
// such as libx265, may not be built into this machine's ffmpeg.
func generateFixture(t *testing.T, dir, name string, args ...string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	full := append(append([]string{"-y", "-v", "error"}, args...), path)

	cmd := exec.CommandContext(t.Context(), "ffmpeg", full...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not generate fixture %s: %v\n%s", name, err, out)
	}

	return path
}

// generateRotatedFixture builds a 1280x720 landscape clip, then
// remuxes it with a 90-degree display-matrix rotation injected
// through ffmpeg's own -display_rotation option. Confirmed on this
// machine that this option is read-side only - passing it directly to
// an encode's output is rejected - so the rotation has to be injected
// on a second, stream-copy pass over an already-encoded plain clip
// (see docs/decisions.md).
func generateRotatedFixture(t *testing.T, dir string) string {
	t.Helper()

	plain := generateFixture(t, dir, "plain.mp4",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:duration=2:rate=30", "-c:v", "libx264")

	rotated := filepath.Join(dir, "rotated.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg",
		"-y", "-v", "error", "-display_rotation:v:0", "90", "-i", plain, "-c", "copy", rotated)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not inject rotation metadata: %v\n%s", err, out)
	}

	return rotated
}

// probeEntry runs the host's own ffprobe and returns the trimmed
// value of one stream entry for the first stream of kind - "v" or
// "a" - in path.
func probeEntry(t *testing.T, path, kind, entry string) string {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH")
	}

	cmd := exec.CommandContext(t.Context(), ffprobePath,
		"-v", "error",
		"-select_streams", kind+":0",
		"-show_entries", "stream="+entry,
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}

	return strings.TrimSpace(string(out))
}
