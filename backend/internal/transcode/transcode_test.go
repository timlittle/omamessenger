package transcode_test

// transcode_test.go tests the shared run/classify/validate logic
// every profile method goes through, black-box, driving it through
// Video: which particular profile method is used does not matter to
// any of these cases, since all five share the same body (see
// transcode.go). Each profile's own file instead tests only what
// makes it different: its own fixed ffmpeg argument list, and its own
// output name, timeout and size cap.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestVideo_UnavailableSandbox(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{availErr: errors.New("test: no bwrap")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrUnavailable) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrUnavailable)
	}
}

func TestVideo_SandboxRunError(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{runErr: errors.New("test: could not start")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if err == nil {
		t.Fatal("Video() error = nil, want an error when the sandbox itself fails to run")
	}
}

func TestVideo_TimedOut(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{TimedOut: true}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrTimeout) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrTimeout)
	}
}

func TestVideo_TooLarge(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{TooLarge: true}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrTooLong) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrTooLong)
	}
}

func TestVideo_FfmpegExitedNonZero(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 1}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_OutputMissingDespiteSuccess(t *testing.T) {
	t.Parallel()

	// ffmpeg reporting success is not, on its own, proof it actually
	// wrote the file this profile looks for: nothing is written here.
	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_OutputEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: []byte{}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_OutputIsASymlink(t *testing.T) {
	t.Parallel()

	// A compromised or misbehaving ffmpeg leaving a symlink where the
	// sandbox told it to write its real output must never be treated
	// as the output itself: whatever the link points at - here, a
	// file the test controls, but it could as easily be /etc/shadow -
	// would otherwise reach whatever this helper does next with "the
	// transcoded file" (serve it, copy it, open it).
	outside := filepath.Join(t.TempDir(), "not-the-output")
	if err := os.WriteFile(outside, []byte("not a video"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputSymlinkTo: outside}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: validMP4Bytes(t, true)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	// The input file's own name carries a sender-chosen shape on
	// purpose: the whole point of this test is that it must never
	// reach the sandbox's own argv (see Spec.Input's own doc comment
	// and docs/decisions.md).
	originalInput := fakeInputFile(t, dir, "attachment-original-name.mp4")

	path, err := tr.Video(t.Context(), originalInput, dir)
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}
	if path == "" {
		t.Fatal("Video() returned an empty path for a successful run")
	}

	if fake.lastSpec.Binary != "/usr/bin/ffmpeg" {
		t.Errorf("Spec.Binary = %q, want /usr/bin/ffmpeg", fake.lastSpec.Binary)
	}
	if fake.lastSpec.Input == originalInput {
		t.Errorf("Spec.Input = %q, the caller's own original path reached the sandbox argv", fake.lastSpec.Input)
	}
	if got := filepath.Base(fake.lastSpec.Input); got != sandbox.InputBaseName {
		t.Errorf("Spec.Input = %q, want a base name of %q", fake.lastSpec.Input, sandbox.InputBaseName)
	}
}

func TestTranscoder_RunsOneJobAtATime(t *testing.T) {
	t.Parallel()

	blocking := newBlockingRunner()
	tr := transcode.NewTranscoder(blocking, "/usr/bin/ffmpeg")

	// First job: starts, reaches Run, and sits there until the test
	// lets it go.
	firstDone := make(chan error, 1)
	go func() {
		dir := t.TempDir()
		_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
		firstDone <- err
	}()

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first job never reached Run")
	}

	// Second job: a different profile, to prove the slot is shared
	// across every profile on this Transcoder, not just repeats of
	// the same one. Its own context ends well before the first job
	// is let go, so it must never reach Run at all - proving the slot,
	// not luck, is what is blocking it.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	secondDir := t.TempDir()
	start := time.Now()
	_, err := tr.Audio(ctx, fakeInputFile(t, secondDir, "in.mp4"), secondDir)
	elapsed := time.Since(start)

	if !errors.Is(err, transcode.ErrTimeout) {
		t.Fatalf("Audio() error = %v, want %v", err, transcode.ErrTimeout)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Audio() took %s to report a 100ms context timeout; it is not honouring ctx while waiting for the slot", elapsed)
	}

	// Letting the first job finish must free the slot for the next
	// one. blockingRunner writes no output file, so the job itself
	// still ends in ErrFailed - only that it actually completes
	// (proving release ran) matters here, not its own outcome.
	close(blocking.unblock)

	select {
	case err := <-firstDone:
		if !errors.Is(err, transcode.ErrFailed) {
			t.Fatalf("first job's Video() error = %v, want %v", err, transcode.ErrFailed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first job never finished after being unblocked")
	}

	// The same Transcoder, same slot: with unblock already closed,
	// Run returns immediately now, so a third call only still reports
	// ErrTimeout, or takes anywhere near as long as acquiring the
	// slot would have, if release never actually ran. blockingRunner
	// writes no output file, so the call still fails - just not with
	// ErrTimeout, and not slowly.
	ctx3, cancel3 := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel3()

	thirdDir := t.TempDir()
	start = time.Now()
	_, err = tr.Audio(ctx3, fakeInputFile(t, thirdDir, "in.mp4"), thirdDir)
	elapsed = time.Since(start)

	if errors.Is(err, transcode.ErrTimeout) {
		t.Fatalf("Audio() after the first job finished = %v, want anything but ErrTimeout - the slot was not freed", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Audio() after the first job finished took %s; the slot was not freed promptly", elapsed)
	}
}

// TestVideo_RemovesTheJobDirectoryOnEveryFailure confirms run cleans
// up the directory it made under outDir on every way a job can fail,
// not just a successful one: a caller only ever removes the one file
// run actually hands back, never a directory it was never told the
// name of, so a failed job that left its own directory behind would
// otherwise never be removed by anything (see app.transcodedPlayCopy
// and docs/decisions.md).
func TestVideo_RemovesTheJobDirectoryOnEveryFailure(t *testing.T) {
	t.Parallel()

	outsideTarget := filepath.Join(t.TempDir(), "not-the-output")
	if err := os.WriteFile(outsideTarget, []byte("not a video"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	cases := []struct {
		name string
		fake *fakeRunner
	}{
		{"sandbox run error", &fakeRunner{runErr: errors.New("test: could not start")}},
		{"timed out", &fakeRunner{result: sandbox.Result{TimedOut: true}}},
		{"too large", &fakeRunner{result: sandbox.Result{TooLarge: true}}},
		{"ffmpeg exited non-zero", &fakeRunner{result: sandbox.Result{ExitCode: 1}}},
		{"output missing despite success", &fakeRunner{result: sandbox.Result{ExitCode: 0}}},
		{"output empty", &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: []byte{}}},
		{"output is a symlink", &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputSymlinkTo: outsideTarget}},
		{"rebuild fails", &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: []byte("not a real mp4")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			tr := transcode.NewTranscoder(c.fake, "/usr/bin/ffmpeg")

			dir := t.TempDir()
			_, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir)
			if err == nil {
				t.Fatal("Video() error = nil, want an error for this failure case")
			}

			jobDir := c.fake.lastSpec.OutputDir
			if jobDir == "" {
				t.Fatal("fake runner never saw a job directory - this case does not exercise the cleanup path")
			}
			if _, statErr := os.Stat(jobDir); !os.IsNotExist(statErr) {
				t.Errorf("job directory %q still exists after a failed run, want it removed", jobDir)
			}
		})
	}
}

// TestVideo_RemovesTheJobDirectoryWhenJobInputCannotBePlaced confirms
// the same cleanup when run fails before ever reaching the sandbox at
// all: placeJobInput cannot link or copy an input file that does not
// exist, so this never builds a Spec or calls fakeRunner.Run, and the
// table above's own check (which reads back the job directory from
// fake.lastSpec) cannot tell whether this path was exercised at all.
func TestVideo_RemovesTheJobDirectoryWhenJobInputCannotBePlaced(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: validMP4Bytes(t, true)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "never-written.mp4")

	_, err := tr.Video(t.Context(), missing, dir)
	if err == nil {
		t.Fatal("Video() error = nil, want an error for an input file that does not exist")
	}
	if fake.lastSpec.OutputDir != "" {
		t.Fatal("fakeRunner.Run was called at all; placeJobInput should have failed first")
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("reading outDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("outDir still holds %v after a failed run, want the job directory removed", entries)
	}
}
