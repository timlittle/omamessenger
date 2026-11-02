package transcode

// video.go runs the Video profile: ffmpeg's own fixed argument list
// for re-encoding a received video into an MP4 the shell may safely
// play. See docs/decisions.md for why each flag is there.

import (
	"context"
	"strconv"
	"time"
)

// videoOutputName is the file Video's own argv writes inside the
// sandbox's output directory, and the one Video itself looks for
// afterwards.
const videoOutputName = "out.mp4"

// videoTimeout bounds a Video job's wall clock: generous next to the
// cost measured re-encoding a 60-second 1080p clip to 720p on this
// machine (around 5 seconds inside the sandbox at two threads),
// scaled up for videoDurationSeconds's own ten-minute cap, while still
// bounding a file that makes ffmpeg hang rather than simply run slowly
// (see docs/decisions.md).
const videoTimeout = 5 * time.Minute

// videoDurationSeconds caps how much of the input Video actually
// encodes (-t): the same ten-minute ceiling videosafe.maxDuration
// already used for a video attachment (this package cannot import
// videosafe; see .golangci.yml's depguard rules).
const videoDurationSeconds = 600

// videoMaxOutputBytes bounds Video's own output file, checked after
// ffmpeg exits: generous next to the roughly 100 MiB a full ten-
// minute clip at this profile's settings should reach, extrapolated
// from the 10 MiB measured for a 60-second clip at a similar bitrate
// (see docs/decisions.md), well under the sandbox's own, blunter
// backstop.
const videoMaxOutputBytes = 200 << 20

// Video re-encodes in - any video ffmpeg can decode - into an MP4 at
// outDir: H.264 (main profile), scaled to fit 1280x1280 without ever
// upscaling, at most 30 frames per second, with its first audio
// stream, if any, as stereo AAC. It returns the output file's path, or
// one of this package's own sentinel errors (see the package doc
// comment).
func (t *Transcoder) Video(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       videoArgs(),
		outputName: videoOutputName,
		timeout:    videoTimeout,
		maxOutput:  videoMaxOutputBytes,
	})
}

// videoArgs is the Video profile's own fixed ffmpeg argument list.
func videoArgs() []string {
	args := commonInputArgs()
	args = append(args, "-map", "0:v:0", "-map", "0:a:0?")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-vf", fitWithinFilter(1280)+",scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-ac", "2", "-b:a", "96k",
		"-t", strconv.Itoa(videoDurationSeconds),
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath(videoOutputName),
	)

	return args
}
