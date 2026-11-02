package transcode_test

// video_test.go checks Video's own fixed ffmpeg argument list,
// captured through fakeRunner (see helpers_test.go); the shared run,
// classify and validate logic every profile method goes through is
// tested once, through Video, in transcode_test.go.

import (
	"slices"
	"strconv"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestVideo_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: validMP4Bytes(t, true)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	if _, err := tr.Video(t.Context(), fakeInputFile(t, dir, "in.mp4"), dir); err != nil {
		t.Fatalf("Video() error = %v", err)
	}

	want := []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", "536870912",
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn",
		"-vf", "scale='min(1280,iw)':'min(1280,ih)':force_original_aspect_ratio=decrease,scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-level:v", "4.0", "-preset", "veryfast", "-crf", "23", "-bf", "0",
		"-maxrate", "2500k", "-bufsize", "5000k",
		"-c:a", "aac", "-ac", "2", "-b:a", "96k",
		"-t", "600",
		"-movflags", "+faststart",
		"-f", "mp4",
		sandbox.OutputDirPath + "/out.mp4",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestVideo_RealEncode_PortraitWithAudio(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "portrait.mp4",
		"-f", "lavfi", "-i", "testsrc2=size=720x1280:duration=2:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-profile:v", "high", "-c:a", "aac", "-shortest")

	out, err := tr.Video(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}

	assertCleanContainer(t, out, true)
	assertWithinBox(t, out, 1280)
}

func TestVideo_RealEncode_Rotated(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateRotatedFixture(t, dir)

	out, err := tr.Video(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}

	// The source is a 1280x720 landscape frame tagged to display
	// rotated 90 degrees; ffmpeg's own autorotate must have physically
	// rotated the decoded frame before this profile's own scale
	// filter ever saw it, so the output is portrait, not landscape,
	// and carries no rotation of its own.
	width := probeInt(t, out, "width")
	height := probeInt(t, out, "height")
	if height <= width {
		t.Errorf("output is %dx%d, want a portrait frame (height > width) for a 90-degree-rotated landscape source", width, height)
	}
	if rotation := probeEntry(t, out, "v", "side_data_list"); rotation != "" {
		t.Errorf("output still carries side data %q, want none: autorotate should have baked the rotation into the pixels", rotation)
	}
	assertCleanContainer(t, out, false)
}

func TestVideo_RealEncode_LargeLandscape(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "large.mp4",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:duration=1:rate=30", "-c:v", "libx264")

	out, err := tr.Video(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}

	assertWithinBox(t, out, 1280)
	assertCleanContainer(t, out, false)
}

func TestVideo_RealEncode_HEVCSource(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	// generateFixture itself skips this test if libx265 is not built
	// into the host's ffmpeg.
	in := generateFixture(t, dir, "hevc.mp4",
		"-f", "lavfi", "-i", "testsrc2=size=640x480:duration=1:rate=30", "-c:v", "libx265")

	out, err := tr.Video(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Video() error = %v", err)
	}

	assertCleanContainer(t, out, false)
}

// assertWithinBox fails t unless path's own video stream fits within
// a max x max box without exceeding it in either dimension.
func assertWithinBox(t *testing.T, path string, max int) {
	t.Helper()

	width := probeInt(t, path, "width")
	height := probeInt(t, path, "height")
	if width > max || height > max {
		t.Errorf("output is %dx%d, want both dimensions within %d", width, height, max)
	}
}

// probeInt runs probeEntry and parses its result as an integer,
// failing t if ffprobe's own output cannot be parsed as one.
func probeInt(t *testing.T, path, entry string) int {
	t.Helper()

	n, err := strconv.Atoi(probeEntry(t, path, "v", entry))
	if err != nil {
		t.Fatalf("parsing ffprobe's own %s for %s: %v", entry, path, err)
	}

	return n
}
