package transcode

// animation.go runs the Animation profile: Video's settings with no
// audio, for a GIF or silent "GIF video" (see docs/decisions.md).

import (
	"context"
	"path/filepath"
	"strconv"
)

// animationOutputName is the file Animation writes in the sandbox's output directory.
const animationOutputName = "out.mp4"

// animationTimeout is Video's, since an Animation encode never costs more.
const animationTimeout = videoTimeout

// animationDurationSeconds is Video's duration cap.
const animationDurationSeconds = videoDurationSeconds

// animationMaxOutputBytes is Video's output cap; with no audio it is never reached sooner.
const animationMaxOutputBytes = videoMaxOutputBytes

// Animation re-encodes an animated GIF, WebP or "GIF video" into a silent MP4 at outDir.
func (t *Transcoder) Animation(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, in, outDir, profile{
		args:       animationArgs(),
		outputName: animationOutputName,
		timeout:    animationTimeout,
		maxOutput:  animationMaxOutputBytes,
		rebuild:    animationRebuild,
	})
}

// animationRebuild validates and rewrites Animation's MP4, rejecting any audio track.
func animationRebuild(sandboxedOutput, jobDir string) (string, error) {
	outPath := filepath.Join(jobDir, "remux.mp4")
	caps := remuxCaps{
		allowAudio:        false,
		maxVideoSamples:   animationDurationSeconds*30 + remuxSampleCountSlack,
		maxVideoSampleLen: maxRemuxVideoSampleBytes,
		maxOutputBytes:    animationMaxOutputBytes,
	}
	if err := remuxMP4(sandboxedOutput, outPath, caps); err != nil {
		return "", err
	}
	return outPath, nil
}

// animationArgs is the Animation profile's ffmpeg argument list.
func animationArgs() []string {
	args := commonInputArgs()
	args = append(args, filterThreadArgs()...)
	args = append(args, "-map", "0:v:0", "-an")
	args = append(args, stripMetadataArgs()...)
	args = append(args,
		"-vf", fitWithinFilter(1280)+",scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-pix_fmt", "yuv420p",
		"-fpsmax", "30",
		"-c:v", "libx264", "-profile:v", "main", "-level:v", "4.0", "-preset", "veryfast", "-crf", "23", "-bf", "0",
	)
	args = append(args, libx264ThreadArgs()...)
	args = append(args, videoRateControlArgs()...)
	args = append(args, outputThreadArgs()...)
	args = append(args,
		"-t", strconv.Itoa(animationDurationSeconds),
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath(animationOutputName),
	)

	return args
}
