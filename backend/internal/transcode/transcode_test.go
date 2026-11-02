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

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if !errors.Is(err, transcode.ErrUnavailable) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrUnavailable)
	}
}

func TestVideo_SandboxRunError(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{runErr: errors.New("test: could not start")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if err == nil {
		t.Fatal("Video() error = nil, want an error when the sandbox itself fails to run")
	}
}

func TestVideo_TimedOut(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{TimedOut: true}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if !errors.Is(err, transcode.ErrTimeout) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrTimeout)
	}
}

func TestVideo_TooLarge(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{TooLarge: true}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if !errors.Is(err, transcode.ErrTooLong) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrTooLong)
	}
}

func TestVideo_FfmpegExitedNonZero(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 1}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
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

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_OutputEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: []byte{}}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
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

	_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if !errors.Is(err, transcode.ErrFailed) {
		t.Fatalf("Video() error = %v, want %v", err, transcode.ErrFailed)
	}
}

func TestVideo_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{result: sandbox.Result{ExitCode: 0}, writeOutputName: "out.mp4", writeOutputData: []byte("video bytes")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	path, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}
	if path == "" {
		t.Fatal("Video() returned an empty path for a successful run")
	}

	if fake.lastSpec.Binary != "/usr/bin/ffmpeg" {
		t.Errorf("Spec.Binary = %q, want /usr/bin/ffmpeg", fake.lastSpec.Binary)
	}
	if fake.lastSpec.Input != "/in.mp4" {
		t.Errorf("Spec.Input = %q, want /in.mp4", fake.lastSpec.Input)
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
		_, err := tr.Video(t.Context(), "/in.mp4", t.TempDir())
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

	start := time.Now()
	_, err := tr.Audio(ctx, "/in.mp4", t.TempDir())
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

	start = time.Now()
	_, err = tr.Audio(ctx3, "/in.mp4", t.TempDir())
	elapsed = time.Since(start)

	if errors.Is(err, transcode.ErrTimeout) {
		t.Fatalf("Audio() after the first job finished = %v, want anything but ErrTimeout - the slot was not freed", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Audio() after the first job finished took %s; the slot was not freed promptly", elapsed)
	}
}
