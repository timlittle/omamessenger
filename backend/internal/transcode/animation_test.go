package transcode_test

// animation_test.go checks Animation's own fixed ffmpeg argument list
// and its own output wiring, captured and written through fakeRunner
// (see helpers_test.go).

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestAnimation_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: validMP4Bytes(t, false)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	if _, err := tr.Animation(t.Context(), fakeInputFile(t, dir, "in.gif"), dir); err != nil {
		t.Fatalf("Animation() error = %v", err)
	}

	want := []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", "536870912",
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
		"-map", "0:v:0", "-an",
		"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn",
		"-vf", "scale='min(1280,iw)':'min(1280,ih)':force_original_aspect_ratio=decrease,scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-level:v", "4.0", "-preset", "veryfast", "-crf", "23", "-bf", "0",
		"-maxrate", "2500k", "-bufsize", "5000k",
		"-t", "600",
		"-movflags", "+faststart",
		"-f", "mp4",
		sandbox.OutputDirPath + "/out.mp4",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestAnimation_RealEncode_GIF(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "anim.gif",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:duration=1:rate=10")

	out, err := tr.Animation(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Animation() error = %v", err)
	}

	assertCleanContainer(t, out, false)
}

func TestAnimation_RealEncode_AnimatedWebP(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "anim.webp",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:duration=1:rate=10", "-c:v", "libwebp", "-loop", "0", "-lossless", "0")

	out, err := tr.Animation(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Animation() error = %v", err)
	}

	assertCleanContainer(t, out, false)
}

func TestAnimation_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: validMP4Bytes(t, false)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	path, err := tr.Animation(t.Context(), fakeInputFile(t, dir, "in.gif"), dir)
	if err != nil {
		t.Fatalf("Animation() error = %v", err)
	}
	if path == "" {
		t.Fatal("Animation() returned an empty path for a successful run")
	}
}
