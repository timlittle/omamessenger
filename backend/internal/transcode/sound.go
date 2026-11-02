package transcode

// sound.go runs the Sound profile: ffmpeg's own fixed argument list
// for re-encoding a short notification or reaction sound into an
// uncompressed WAV file, the only format Qt's SoundEffect can play
// (QTBUG and the Qt Multimedia documentation both confirm it decodes
// no compressed codec itself). See docs/decisions.md for why each
// flag is there.

import (
	"context"
	"strconv"
	"time"
)

// soundOutputName is the file Sound's own argv writes inside the
// sandbox's output directory.
const soundOutputName = "out.wav"

// soundTimeout bounds a Sound job's wall clock: PCM encoding is
// close to free, and soundDurationSeconds already caps the input to
// 30 seconds, so this only needs room for ffmpeg's own startup and
// probing.
const soundTimeout = 45 * time.Second

// soundDurationSeconds caps how much of the input Sound actually
// encodes (-t): Qt's SoundEffect is for short notification and
// reaction sounds, never a voice note (see Audio).
const soundDurationSeconds = 30

// soundMaxOutputBytes bounds Sound's own output file: uncompressed
// stereo PCM at 48 kHz for soundDurationSeconds is exactly 48000 * 2
// channels * 2 bytes per sample * 30 seconds = 5,760,000 bytes; this
// leaves modest headroom above that exact figure.
const soundMaxOutputBytes = 8 << 20

// Sound re-encodes in's first audio stream into an uncompressed WAV
// file at outDir: stereo, 48 kHz, 16-bit PCM, no video, capped at 30
// seconds. It returns the output file's path, or one of this
// package's own sentinel errors (see the package doc comment).
func (t *Transcoder) Sound(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       soundArgs(),
		outputName: soundOutputName,
		timeout:    soundTimeout,
		maxOutput:  soundMaxOutputBytes,
	})
}

// soundArgs is the Sound profile's own fixed ffmpeg argument list.
func soundArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:a:0", "-vn")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-c:a", "pcm_s16le", "-ac", "2", "-ar", "48000",
		"-t", strconv.Itoa(soundDurationSeconds),
		"-f", "wav",
		outputPath(soundOutputName),
	)

	return args
}
