package transcode

// animation.go runs the Animation profile: the same video settings
// Video uses, but with no audio at all, for a GIF or a "GIF video" -
// a short, silent clip a messaging service marks to autoplay. See
// docs/decisions.md for why each flag is there.

import (
	"context"
	"strconv"
)

// animationOutputName is the file Animation's own argv writes inside
// the sandbox's output directory.
const animationOutputName = "out.mp4"

// animationTimeout reuses videoTimeout: Animation runs the same
// video encode Video does, minus audio, so it can never cost more.
const animationTimeout = videoTimeout

// animationDurationSeconds reuses videoDurationSeconds: a "GIF video"
// is a video by another name, so the same ten-minute cap applies.
const animationDurationSeconds = videoDurationSeconds

// animationMaxOutputBytes bounds Animation's own output file: smaller
// than videoMaxOutputBytes, since dropping the audio stream a video
// of the same length and dimensions would otherwise carry removes a
// meaningful share of it, while a real GIF or "GIF video" is short
// enough in practice to stay well inside either cap.
const animationMaxOutputBytes = 100 << 20

// Animation re-encodes in - an animated GIF, WebP, or a "GIF video" -
// into a silent MP4 at outDir, with the same scaling, frame-rate and
// video codec settings Video uses. It returns the output file's path,
// or one of this package's own sentinel errors (see the package doc
// comment).
func (t *Transcoder) Animation(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       animationArgs(),
		outputName: animationOutputName,
		timeout:    animationTimeout,
		maxOutput:  animationMaxOutputBytes,
	})
}

// animationArgs is the Animation profile's own fixed ffmpeg argument
// list.
func animationArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:v:0", "-an")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-vf", fitWithinFilter(1280)+",scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-preset", "veryfast", "-crf", "23",
		"-t", strconv.Itoa(animationDurationSeconds),
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath(animationOutputName),
	)

	return args
}
