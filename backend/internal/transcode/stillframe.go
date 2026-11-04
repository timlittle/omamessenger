package transcode

// stillframe.go extracts a static PNG thumbnail from a video or
// animation, including animated WebP stickers Go's own image packages
// cannot decode. See docs/decisions.md for why each ffmpeg flag is there.

import (
	"context"
	"time"
)

// stillFrameOutputName is the file StillFrame's own argv writes
// inside the sandbox's output directory.
const stillFrameOutputName = "out.png"

// stillFrameTimeout bounds a StillFrame job's wall clock: decoding one
// frame is far cheaper than a full encode.
const stillFrameTimeout = 30 * time.Second

// stillFrameMaxOutputBytes bounds StillFrame's PNG output, generous
// even for a frame with hard-to-compress detail.
const stillFrameMaxOutputBytes = 4 << 20

// StillFrame extracts in's first video frame as a PNG at outDir,
// scaled to fit 1024x1024 without upscaling.
func (t *Transcoder) StillFrame(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       stillFrameArgs(),
		outputName: stillFrameOutputName,
		timeout:    stillFrameTimeout,
		maxOutput:  stillFrameMaxOutputBytes,
	})
}

// stillFrameArgs is StillFrame's fixed ffmpeg argument list. Thread
// args cap the scale filter and png encoder, which otherwise default
// to this machine's full core count.
func stillFrameArgs() []string {
	args := commonInputArgs()
	args = append(args, filterThreadArgs()...)
	args = append(args, "-map", "0:v:0", "-an")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-vframes", "1",
		"-vf", fitWithinFilter(1024),
		"-c:v", "png",
	)
	args = append(args, outputThreadArgs()...)
	args = append(args,
		"-f", "image2",
		outputPath(stillFrameOutputName),
	)

	return args
}
