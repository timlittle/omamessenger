package transcode

// video.go runs the Video profile: ffmpeg's own fixed argument list
// for re-encoding a received video into an MP4 the shell may safely
// play. See docs/decisions.md for why each flag is there.

import (
	"context"
	"path/filepath"
	"strconv"
	"time"
)

// videoOutputName is the file Video's own argv writes inside the
// sandbox's output directory, and the one Video itself looks for
// afterwards.
const videoOutputName = "out.mp4"

// videoTimeout bounds a Video job's wall clock: generous next to the
// cost measured re-encoding a 60-second 1080p clip to 720p (around 5
// seconds inside the sandbox at two threads), scaled up for
// videoDurationSeconds's own ten-minute cap, while still bounding a
// file that makes ffmpeg hang rather than simply run slowly
// (see docs/decisions.md).
const videoTimeout = 5 * time.Minute

// videoDurationSeconds caps how much of the input Video actually
// encodes (-t): the same ten-minute ceiling videosafe.maxDuration
// already used for a video attachment (this package cannot import
// videosafe; see .golangci.yml's depguard rules).
const videoDurationSeconds = 600

// videoMaxOutputBytes bounds Video's own output file, checked after
// ffmpeg exits: videoRateControlArgs' own VBV cap bounds the worst
// case at roughly 194.7 MB (600s * (2500+96) kbit/s / 8) regardless of
// how adversarial the input is, so this leaves a margin above that -
// not the looser, typical-content estimate an unbounded CRF encode
// would otherwise need - well under the sandbox's own, blunter
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
		rebuild:    videoRebuild,
	})
}

// videoRemuxCaps bounds what videoRebuild's own call to remuxMP4
// will accept: sample counts sized for Video's own duration cap at
// 30 frames per second of video and, generously, 50 AAC-LC frames
// per second of audio, each with a little slack (see remux.go).
func videoRemuxCaps() remuxCaps {
	return remuxCaps{
		allowAudio:        true,
		maxVideoSamples:   videoDurationSeconds*30 + remuxSampleCountSlack,
		maxAudioSamples:   videoDurationSeconds*maxAACFramesPerSecond + remuxSampleCountSlack,
		maxVideoSampleLen: maxRemuxVideoSampleBytes,
		maxAudioSampleLen: maxRemuxAudioSampleBytes,
	}
}

// videoRebuild validates the MP4 ffmpeg wrote for Video and
// rewrites it as sandboxedOutput's own replacement in jobDir (see
// remuxMP4): a hijacked ffmpeg can then only ever reach Qt's own
// H.264 and AAC decoders, through a container this package wrote.
func videoRebuild(sandboxedOutput, jobDir string) (string, error) {
	outPath := filepath.Join(jobDir, "remux.mp4")
	if err := remuxMP4(sandboxedOutput, outPath, videoRemuxCaps()); err != nil {
		return "", err
	}
	return outPath, nil
}

// videoArgs is the Video profile's own fixed ffmpeg argument list.
func videoArgs() []string {
	args := commonInputArgs()
	args = append(args, filterThreadArgs()...)
	args = append(args, "-map", "0:v:0", "-map", "0:a:0?")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-vf", fitWithinFilter(1280)+",scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-level:v", "4.0", "-preset", "veryfast", "-crf", "23", "-bf", "0",
	)
	args = append(args, libx264ThreadArgs()...)
	args = append(args, videoRateControlArgs()...)
	args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "96k")
	args = append(args, outputThreadArgs()...)
	args = append(args,
		"-t", strconv.Itoa(videoDurationSeconds),
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath(videoOutputName),
	)

	return args
}
