package transcode_test

// transcode_test.go tests the shared run/classify/validate logic
// every profile method goes through, black-box, driving it through
// Video: which particular profile method is used does not matter to
// any of these cases, since all five share the same body (see
// transcode.go). Each profile's own file instead tests only what
// makes it different: its own fixed ffmpeg argument list, and its own
// output name, timeout and size cap.

import (
	"errors"
	"testing"

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
