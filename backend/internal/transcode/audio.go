package transcode

// audio.go runs the Audio profile: re-encodes a voice note or audio
// message into a WAV file this package rebuilds before the shell sees
// it (see docs/decisions.md).

import (
	"context"
	"path/filepath"
	"strconv"
	"time"
)

// audioOutputName is the file Audio's own argv writes inside the
// sandbox's output directory.
const audioOutputName = "out.wav"

// audioSampleRate is Audio's own WAV sample rate: 24 kHz is enough
// for speech and keeps a voice note small.
const audioSampleRate = 24000

// audioTimeout bounds an Audio job's wall clock: PCM encoding costs
// far less than Video's H.264 encode, so this stays well under
// videoTimeout.
const audioTimeout = 2 * time.Minute

// audioDurationSeconds reuses videoDurationSeconds, generous for any
// real voice note.
const audioDurationSeconds = videoDurationSeconds

// audioMaxOutputBytes bounds Audio's output file, with headroom above
// the exact size of ten minutes of uncompressed stereo PCM at
// audioSampleRate.
const audioMaxOutputBytes = 64 << 20

// Audio re-encodes in's first audio stream into an uncompressed WAV
// file at outDir: mono stays mono, anything wider is downmixed to
// stereo, at audioSampleRate, 16-bit PCM, no video.
func (t *Transcoder) Audio(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       audioArgs(),
		outputName: audioOutputName,
		timeout:    audioTimeout,
		maxOutput:  audioMaxOutputBytes,
		rebuild:    audioRebuild,
	})
}

// audioArgs is the Audio profile's own fixed ffmpeg argument list.
func audioArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:a:0", "-vn")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-af", pcmFormatFilter,
		"-ar", strconv.Itoa(audioSampleRate),
		"-c:a", "pcm_s16le", "-bitexact",
		"-t", strconv.Itoa(audioDurationSeconds),
		"-f", "wav",
		outputPath(audioOutputName),
	)

	return args
}

// audioRebuild validates and rewrites the WAV ffmpeg wrote for Audio,
// so no byte of the container that reaches the shell comes from the
// sandbox (see rebuildWAV).
func audioRebuild(sandboxedOutput, jobDir string) (string, error) {
	finalPath := filepath.Join(jobDir, "final.wav")
	if err := rebuildWAV(sandboxedOutput, finalPath, audioSampleRate, audioMaxOutputBytes); err != nil {
		return "", err
	}
	return finalPath, nil
}
