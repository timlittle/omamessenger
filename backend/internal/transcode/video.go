package transcode

// video.go runs the Video profile: re-encodes a received video into
// an MP4 the shell may safely play (see docs/decisions.md).

import (
	"context"
	"path/filepath"
	"strconv"
	"time"
)

// videoOutputName is the file Video's argv writes in the sandbox's
// output directory.
const videoOutputName = "out.mp4"

// videoTimeout bounds a Video job's wall clock, generous next to the
// cost measured re-encoding a 1080p clip at videoDurationSeconds's
// scale, while still catching a file that makes ffmpeg hang.
const videoTimeout = 5 * time.Minute

// videoDurationSeconds caps how much of the input Video encodes (-t),
// matching videosafe.maxDuration's own cap on a video attachment.
const videoDurationSeconds = 600

// videoMaxOutputBytes bounds Video's output file, checked after
// ffmpeg exits: a margin above the worst case videoRateControlArgs'
// VBV cap already guarantees.
const videoMaxOutputBytes = 200 << 20

// Video re-encodes in - any video ffmpeg can decode - into an MP4 at
// outDir: H.264 (main profile), scaled to fit 1280x1280 without
// upscaling, at most 30 fps, with its first audio stream as stereo
// AAC.
func (t *Transcoder) Video(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       videoArgs(),
		outputName: videoOutputName,
		timeout:    videoTimeout,
		maxOutput:  videoMaxOutputBytes,
		rebuild:    videoRebuild,
	})
}

// videoRemuxCaps bounds what videoRebuild's call to remuxMP4 will
// accept: sample counts sized for Video's own duration cap, with a
// little slack (see remux.go), and the output capped at
// videoMaxOutputBytes.
func videoRemuxCaps() remuxCaps {
	return remuxCaps{
		allowAudio:        true,
		maxVideoSamples:   videoDurationSeconds*30 + remuxSampleCountSlack,
		maxAudioSamples:   videoDurationSeconds*maxAACFramesPerSecond + remuxSampleCountSlack,
		maxVideoSampleLen: maxRemuxVideoSampleBytes,
		maxAudioSampleLen: maxRemuxAudioSampleBytes,
		maxOutputBytes:    videoMaxOutputBytes,
	}
}

// videoRebuild validates and rewrites Video's MP4, so a hijacked ffmpeg
// reaches the shell only through a container this package wrote.
func videoRebuild(sandboxedOutput, jobDir string) (string, error) {
	outPath := filepath.Join(jobDir, "remux.mp4")
	if err := remuxMP4(sandboxedOutput, outPath, videoRemuxCaps()); err != nil {
		return "", err
	}
	return outPath, nil
}

// videoArgs is the Video profile's ffmpeg argument list.
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
