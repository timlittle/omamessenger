package transcode_test

// audio_test.go checks Audio's own fixed ffmpeg argument list and its
// own output wiring, captured and written through fakeRunner (see
// helpers_test.go).

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestAudio_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutputName: "out.ogg"}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	if _, err := tr.Audio(t.Context(), "/in.m4a", t.TempDir()); err != nil {
		t.Fatalf("Audio() error = %v", err)
	}

	want := []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", "536870912",
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
		"-map", "0:a:0", "-vn",
		"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn",
		"-c:a", "libopus", "-ac", "2", "-ar", "48000", "-b:a", "64k",
		"-t", "600",
		"-f", "ogg",
		sandbox.OutputDirPath + "/out.ogg",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestAudio_RealEncode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		args []string
	}{
		"opus voice note": {name: "voice.opus", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2"}},
		"m4a":             {name: "voice.m4a", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "aac"}},
		"mp3":             {name: "voice.mp3", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "libmp3lame"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tr := requireRealTranscoder(t)
			dir := integrationDir(t)

			in := generateFixture(t, dir, tt.name, tt.args...)

			out, err := tr.Audio(t.Context(), in, dir)
			if err != nil {
				t.Fatalf("Audio() error = %v", err)
			}

			if codec := probeEntry(t, out, "a", "codec_name"); codec != "opus" {
				t.Errorf("audio codec = %q, want opus", codec)
			}
			if codec := probeEntry(t, out, "v", "codec_name"); codec != "" {
				t.Errorf("video codec = %q, want no video stream at all", codec)
			}
		})
	}
}

func TestAudio_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutputName: "out.ogg", writeOutputData: []byte("opus bytes")}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	path, err := tr.Audio(t.Context(), "/in.m4a", t.TempDir())
	if err != nil {
		t.Fatalf("Audio() error = %v", err)
	}
	if path == "" {
		t.Fatal("Audio() returned an empty path for a successful run")
	}
}
