package transcode

// stillframe.go runs the StillFrame profile: ffmpeg's own fixed
// argument list for extracting a static PNG from an animated WebP
// sticker (which Go's standard library and x/image cannot decode at
// all) or any other video or animation, for a thumbnail or a preview
// before the user has asked to play it. See docs/decisions.md for why
// each flag is there.

import (
	"context"
	"time"
)

// stillFrameOutputName is the file StillFrame's own argv writes
// inside the sandbox's output directory.
const stillFrameOutputName = "out.png"

// stillFrameTimeout bounds a StillFrame job's wall clock: decoding a
// single frame is far cheaper than any full encode this package runs,
// so this only needs room for ffmpeg's own startup and probing plus a
// handful of frames' worth of decoding to reach the first one.
const stillFrameTimeout = 30 * time.Second

// stillFrameMaxOutputBytes bounds StillFrame's own output file: a
// single PNG frame scaled to fit 1024x1024, generous even for a
// frame with large areas of fine, hard-to-compress detail.
const stillFrameMaxOutputBytes = 4 << 20

// StillFrame extracts in's first video frame - any video, animation
// or animated image ffmpeg can decode - as a PNG at outDir, scaled to
// fit 1024x1024 without ever upscaling. It returns the output file's
// path, or one of this package's own sentinel errors (see the package
// doc comment).
func (t *Transcoder) StillFrame(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       stillFrameArgs(),
		outputName: stillFrameOutputName,
		timeout:    stillFrameTimeout,
		maxOutput:  stillFrameMaxOutputBytes,
	})
}

// stillFrameArgs is the StillFrame profile's own fixed ffmpeg
// argument list: filterThreadArgs and outputThreadArgs bound its own
// scale filter and its png encoder the same way every other profile
// bounds its own filter and encoder (see mediaThreads' own doc
// comment in args.go) - both otherwise default to this machine's own
// core count (confirmed directly: without them, a real animated WebP
// sticker through this exact profile needed prlimit --nproc above 52
// to succeed at all, nothing like the handful of threads every other
// profile's own fixed, capped pipeline needs).
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
