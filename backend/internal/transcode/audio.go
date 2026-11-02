package transcode

// audio.go runs the Audio profile: ffmpeg's own fixed argument list
// for re-encoding a received voice note into an Ogg/Opus file the
// shell may safely play. See docs/decisions.md for why each flag is
// there.

import (
	"context"
	"strconv"
	"time"
)

// audioOutputName is the file Audio's own argv writes inside the
// sandbox's output directory.
const audioOutputName = "out.ogg"

// audioTimeout bounds an Audio job's wall clock: an Opus encode is
// far cheaper than Video's own H.264 encode, so this stays well under
// videoTimeout despite sharing its ten-minute duration cap.
const audioTimeout = 2 * time.Minute

// audioDurationSeconds reuses videoDurationSeconds: the same ten-
// minute ceiling is generous for any real voice note.
const audioDurationSeconds = videoDurationSeconds

// audioMaxOutputBytes bounds Audio's own output file: double the
// roughly 4.8 MiB a full ten-minute clip at 64 kbit/s should reach.
const audioMaxOutputBytes = 10 << 20

// Audio re-encodes in's first audio stream - any audio ffmpeg can
// decode - into an Ogg/Opus file at outDir: stereo, 48 kHz, 64 kbit/s,
// no video. It returns the output file's path, or one of this
// package's own sentinel errors (see the package doc comment).
func (t *Transcoder) Audio(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       audioArgs(),
		outputName: audioOutputName,
		timeout:    audioTimeout,
		maxOutput:  audioMaxOutputBytes,
	})
}

// audioArgs is the Audio profile's own fixed ffmpeg argument list.
func audioArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:a:0", "-vn")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-c:a", "libopus", "-ac", "2", "-ar", "48000", "-b:a", "64k",
		"-t", strconv.Itoa(audioDurationSeconds),
		"-f", "ogg",
		outputPath(audioOutputName),
	)

	return args
}
