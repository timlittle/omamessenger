package transcode_test

// audio_test.go checks Audio's own fixed ffmpeg argument list and
// its own output wiring, captured and written through fakeRunner
// (see helpers_test.go). Audio's own rebuild step - validating and
// rewriting the WAV ffmpeg wrote - is tested once, directly, in
// wav_test.go; these tests only confirm Audio wires that step in
// with its own sample rate and size cap.

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

func TestAudio_Args(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: validWAVBytes(1, 24000, 10)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	if _, err := tr.Audio(t.Context(), fakeInputFile(t, dir, "in.m4a"), dir); err != nil {
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
		"-af", "aformat=sample_fmts=s16:channel_layouts=mono|stereo",
		"-ar", "24000",
		"-c:a", "pcm_s16le", "-bitexact",
		"-t", "600",
		"-f", "wav",
		sandbox.OutputDirPath + "/out.wav",
	}

	if !slices.Equal(fake.lastSpec.Args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", fake.lastSpec.Args, want)
	}
}

func TestAudio_RealEncode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name         string
		args         []string
		wantChannels string
	}{
		"opus voice note": {
			name: "voice.opus", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2"},
			wantChannels: "1",
		},
		"m4a": {
			name: "voice.m4a", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "aac"},
			wantChannels: "1",
		},
		"mp3": {
			name: "voice.mp3", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "libmp3lame"},
			wantChannels: "1",
		},
		"5.1": {
			name: "voice.wav", args: []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-ac", "6"},
			wantChannels: "2", // downmixed by pcmFormatFilter: anything wider than stereo lands as stereo
		},
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

			if codec := probeEntry(t, out, "a", "codec_name"); codec != "pcm_s16le" {
				t.Errorf("audio codec = %q, want pcm_s16le (uncompressed, rebuilt by this package itself)", codec)
			}
			if rate := probeEntry(t, out, "a", "sample_rate"); rate != "24000" {
				t.Errorf("sample rate = %q, want 24000", rate)
			}
			if channels := probeEntry(t, out, "a", "channels"); channels != tt.wantChannels {
				t.Errorf("channels = %q, want %q", channels, tt.wantChannels)
			}
			if codec := probeEntry(t, out, "v", "codec_name"); codec != "" {
				t.Errorf("video codec = %q, want no video stream at all", codec)
			}
		})
	}
}

func TestAudio_Succeeds(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{writeOutput: true, writeOutputData: validWAVBytes(2, 24000, 100)}
	tr := transcode.NewTranscoder(fake, "/usr/bin/ffmpeg")

	dir := t.TempDir()
	path, err := tr.Audio(t.Context(), fakeInputFile(t, dir, "in.m4a"), dir)
	if err != nil {
		t.Fatalf("Audio() error = %v", err)
	}
	if path == "" {
		t.Fatal("Audio() returned an empty path for a successful run")
	}
}
