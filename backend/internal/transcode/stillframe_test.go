package transcode_test

// stillframe_test.go checks StillFrame's own fixed ffmpeg argument
// list and its own output wiring, captured and written through
// fakeRunner (see helpers_test.go).

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestStillFrame_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	if _, err := tr.StillFrame(t.Context(), fakeInputFile(t, dir, "in.webp"), dir); err != nil {
		t.Fatalf("StillFrame() error = %v", err)
	}

	want := []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", "536870912",
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
		"-filter_threads", "2",
		"-map", "0:v:0", "-an",
		"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn",
		"-vframes", "1",
		"-vf", "scale='min(1024,iw)':'min(1024,ih)':force_original_aspect_ratio=decrease",
		"-c:v", "png",
		"-threads", "2",
		"-f", "image2",
		sandbox.OutputDirPath + "/out.png",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestStillFrame_RealEncode_AnimatedWebP(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	// Animated WebP is exactly what this profile exists for: neither
	// Go's standard library nor x/image/webp can decode it at all.
	in := generateFixture(t, dir, "sticker.webp",
		"-f", "lavfi", "-i", "testsrc2=size=512x512:duration=1:rate=10", "-c:v", "libwebp", "-loop", "0", "-lossless", "0")

	out, err := tr.StillFrame(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("StillFrame() error = %v", err)
	}

	// A PNG written by image2 is inherently a single still image; the
	// codec itself is the proof there is exactly one frame, not a
	// frame count ffprobe reports inconsistently for this muxer.
	if codec := probeEntry(t, out, "v", "codec_name"); codec != "png" {
		t.Errorf("codec = %q, want png", codec)
	}
}

func TestStillFrame_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: []byte("png bytes")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	path, err := tr.StillFrame(t.Context(), fakeInputFile(t, dir, "in.webp"), dir)
	if err != nil {
		t.Fatalf("StillFrame() error = %v", err)
	}
	if path == "" {
		t.Fatal("StillFrame() returned an empty path for a successful run")
	}
}
