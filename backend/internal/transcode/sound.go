package transcode

// sound.go runs the Sound profile: ffmpeg's own fixed argument list
// for re-encoding a short notification or reaction sound into an
// uncompressed WAV file this package itself rebuilds before the
// shell ever sees it - the only format Qt's SoundEffect can play
// (QTBUG and the Qt Multimedia documentation both confirm it decodes
// no compressed codec itself). See docs/decisions.md for why each
// flag is there.

import (
	"context"
	"path/filepath"
	"strconv"
	"time"
)

// soundOutputName is the file Sound's own argv writes inside the
// sandbox's output directory.
const soundOutputName = "out.wav"

// soundSampleRate is Sound's own WAV sample rate.
const soundSampleRate = 48000

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
// stereo PCM at soundSampleRate for soundDurationSeconds is exactly
// 48000 * 2 channels * 2 bytes per sample * 30 seconds = 5,760,000
// bytes; this leaves modest headroom above that exact figure.
const soundMaxOutputBytes = 8 << 20

// Sound re-encodes in's first audio stream into an uncompressed WAV
// file at outDir: mono stays mono, anything wider is downmixed to
// stereo, at soundSampleRate, 16-bit PCM, no video, capped at 30
// seconds. It returns the output file's path, or one of this
// package's own sentinel errors (see the package doc comment).
func (t *Transcoder) Sound(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       soundArgs(),
		outputName: soundOutputName,
		timeout:    soundTimeout,
		maxOutput:  soundMaxOutputBytes,
		rebuild:    soundRebuild,
	})
}

// soundArgs is the Sound profile's own fixed ffmpeg argument list.
func soundArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:a:0", "-vn")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-af", pcmFormatFilter,
		"-ar", strconv.Itoa(soundSampleRate),
		"-c:a", "pcm_s16le", "-bitexact",
		"-t", strconv.Itoa(soundDurationSeconds),
		"-f", "wav",
		outputPath(soundOutputName),
	)

	return args
}

// soundRebuild validates the WAV ffmpeg wrote for Sound and rewrites
// it as sandboxedOutput's own replacement in jobDir (see
// rebuildWAV).
func soundRebuild(sandboxedOutput, jobDir string) (string, error) {
	finalPath := filepath.Join(jobDir, "final.wav")
	if err := rebuildWAV(sandboxedOutput, finalPath, soundSampleRate, soundMaxOutputBytes); err != nil {
		return "", err
	}
	return finalPath, nil
}
