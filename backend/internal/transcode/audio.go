package transcode

// audio.go runs the Audio profile: ffmpeg's own fixed argument list
// for re-encoding a received voice note or audio message into a WAV
// file this package itself rebuilds before the shell ever sees it.
// See docs/decisions.md for why each flag is there, and why the
// helper rebuilds the file rather than trusting ffmpeg's own output
// directly.

import (
	"context"
	"path/filepath"
	"strconv"
	"time"
)

// audioOutputName is the file Audio's own argv writes inside the
// sandbox's output directory.
const audioOutputName = "out.wav"

// audioSampleRate is Audio's own WAV sample rate: 24 kHz is more
// than enough for speech, and keeps a voice note small (about
// 2.9 MiB per minute mono, 5.8 MiB stereo of uncompressed PCM).
const audioSampleRate = 24000

// audioTimeout bounds an Audio job's wall clock: PCM encoding costs
// far less than Video's own H.264 encode, so this stays well under
// videoTimeout despite sharing its ten-minute duration cap.
const audioTimeout = 2 * time.Minute

// audioDurationSeconds reuses videoDurationSeconds: the same ten-
// minute ceiling is generous for any real voice note.
const audioDurationSeconds = videoDurationSeconds

// audioMaxOutputBytes bounds Audio's own output file: ten minutes of
// uncompressed stereo PCM at audioSampleRate is exactly
// 24000 * 2 channels * 2 bytes per sample * 600 seconds = 57,600,000
// bytes; 64 MiB leaves modest headroom above that exact figure.
const audioMaxOutputBytes = 64 << 20

// Audio re-encodes in's first audio stream - any audio ffmpeg can
// decode - into an uncompressed WAV file at outDir: mono stays mono,
// anything wider is downmixed to stereo, at audioSampleRate, 16-bit
// PCM, no video. It returns the output file's path, or one of this
// package's own sentinel errors (see the package doc comment).
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

// audioRebuild validates the WAV ffmpeg wrote for Audio and rewrites
// it as sandboxedOutput's own replacement in jobDir (see
// rebuildWAV): no byte of the container that reaches the shell comes
// from the sandbox.
func audioRebuild(sandboxedOutput, jobDir string) (string, error) {
	finalPath := filepath.Join(jobDir, "final.wav")
	if err := rebuildWAV(sandboxedOutput, finalPath, audioSampleRate, audioMaxOutputBytes); err != nil {
		return "", err
	}
	return finalPath, nil
}
