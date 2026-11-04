package transcode

// sound.go runs the Sound profile: re-encoding a short notification
// or reaction sound into uncompressed WAV, the only format Qt's
// SoundEffect can play (see docs/decisions.md).

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

// soundTimeout bounds a Sound job's wall clock.
const soundTimeout = 45 * time.Second

// soundDurationSeconds caps how much of the input Sound encodes (-t).
const soundDurationSeconds = 30

// soundMaxOutputBytes bounds Sound's output file, with headroom above
// the exact size of 30 seconds of stereo PCM at soundSampleRate.
const soundMaxOutputBytes = 8 << 20

// Sound re-encodes in's first audio stream into an uncompressed WAV
// file at outDir, downmixed to stereo at soundSampleRate and capped
// at soundDurationSeconds.
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
// it as sandboxedOutput's replacement in jobDir.
func soundRebuild(sandboxedOutput, jobDir string) (string, error) {
	finalPath := filepath.Join(jobDir, "final.wav")
	if err := rebuildWAV(sandboxedOutput, finalPath, soundSampleRate, soundMaxOutputBytes); err != nil {
		return "", err
	}
	return finalPath, nil
}
