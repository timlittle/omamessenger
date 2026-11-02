package transcode_test

// sound_test.go checks Sound's own fixed ffmpeg argument list and its
// own output wiring, captured and written through fakeRunner (see
// helpers_test.go).

import (
	"slices"
	"strconv"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestSound_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutputName: "out.wav"}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	if _, err := tr.Sound(t.Context(), "/in.ogg", t.TempDir()); err != nil {
		t.Fatalf("Sound() error = %v", err)
	}

	want := []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", "536870912",
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
		"-map", "0:a:0", "-vn",
		"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn",
		"-c:a", "pcm_s16le", "-ac", "2", "-ar", "48000",
		"-t", "30",
		"-f", "wav",
		sandbox.OutputDirPath + "/out.wav",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestSound_RealEncode_UncompressedWAV(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "reaction.ogg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1")

	out, err := tr.Sound(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Sound() error = %v", err)
	}

	if codec := probeEntry(t, out, "a", "codec_name"); codec != "pcm_s16le" {
		t.Errorf("audio codec = %q, want pcm_s16le (uncompressed): Qt's SoundEffect cannot decode anything else", codec)
	}
}

func TestSound_RealEncode_CapsDurationAt30Seconds(t *testing.T) {
	t.Parallel()

	tr := requireRealTranscoder(t)
	dir := integrationDir(t)

	in := generateFixture(t, dir, "long.ogg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=35")

	out, err := tr.Sound(t.Context(), in, dir)
	if err != nil {
		t.Fatalf("Sound() error = %v", err)
	}

	duration, err := strconv.ParseFloat(probeEntry(t, out, "a", "duration"), 64)
	if err != nil {
		t.Fatalf("parsing output duration: %v", err)
	}
	if duration > 31 {
		t.Errorf("output duration = %.1fs, want at most ~30s for a 35-second input", duration)
	}
}

func TestSound_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutputName: "out.wav", writeOutputData: []byte("wav bytes")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	path, err := tr.Sound(t.Context(), "/in.ogg", t.TempDir())
	if err != nil {
		t.Fatalf("Sound() error = %v", err)
	}
	if path == "" {
		t.Fatal("Sound() returned an empty path for a successful run")
	}
}
