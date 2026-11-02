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

// animationMaxOutputBytes reuses videoMaxOutputBytes: Animation
// shares Video's own VBV-capped video settings exactly, minus audio,
// so its own worst case (600s * 2500 kbit/s / 8 ≈ 187.5 MB - see
// videoRateControlArgs) is already lower than Video's own, and does
// not need a tighter cap of its own.
const animationMaxOutputBytes = videoMaxOutputBytes

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
	)
	args = append(args, videoRateControlArgs()...)
	args = append(args,
		"-t", strconv.Itoa(animationDurationSeconds),
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath(animationOutputName),
	)

	return args
}
