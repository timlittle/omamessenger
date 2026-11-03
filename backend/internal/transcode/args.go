package transcode

// args.go holds the pieces of ffmpeg's own argument list every
// profile shares, so each profile file only has to add what makes it
// different.

import (
	"fmt"
	"strconv"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// maxSingleAllocBytes bounds any one allocation ffmpeg's own avutil
// will make (-max_alloc), well under sandbox's own address-space
// limit: no single buffer this package's fixed, bounded profiles ever
// need should come anywhere near this, so a crafted file's declared
// dimensions or bitrate trying to make one hits this long before the
// sandbox's own, blunter backstop.
const maxSingleAllocBytes = 512 << 20

// mediaThreads bounds how many threads this package ever lets any
// one piece of ffmpeg's own pipeline use on its own: the decoder
// (commonInputArgs' own -threads, below), a profile's own filters
// (filterThreadArgs), or its own encoder (outputThreadArgs and
// libx264ThreadArgs). The sandbox this package runs every job inside
// gives it no CPU affinity of its own, only a total CPU-time budget
// (see sandbox.maxCPUSeconds) and the lowest scheduling priority this
// helper can ask for (see sandbox's own niceLevel and ioClass) - so
// without a cap at every one of these points, ffmpeg is otherwise
// free to size its own thread pools by this machine's core count and
// keep all of them busy for the whole length of a job, competing
// with the shell this plugin runs inside for every one of them (see
// docs/decisions.md).
const mediaThreads = 2

// commonInputArgs returns the input-side arguments every profile
// gives ffmpeg, in order: no interactive prompts, overwrite its
// output without asking, log only errors, mediaThreads decode
// threads, this package's own single-allocation cap, and the one
// input file, read only as a local file (-protocol_whitelist file)
// so a crafted container cannot make ffmpeg's own demuxer reach out
// to anything else, a network address included.
func commonInputArgs() []string {
	return []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", strconv.Itoa(mediaThreads),
		"-max_alloc", strconv.Itoa(maxSingleAllocBytes),
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
	}
}

// filterThreadArgs returns the -filter_threads argument a profile
// whose own -vf or -af filter graph could otherwise spread across
// every core this machine has gives ffmpeg: it bounds the thread pool
// ffmpeg's own generic filtergraph framework hands a filter such as
// Video's and Animation's own chained scale, at mediaThreads, the
// same number commonInputArgs already gives the decoder.
func filterThreadArgs() []string {
	return []string{"-filter_threads", strconv.Itoa(mediaThreads)}
}

// outputThreadArgs returns the -threads argument a profile gives
// ffmpeg on the output side, after its own codec and filter options:
// unlike the identical-looking flag commonInputArgs gives on the
// input side, which only ever bounds the decoder reading the input,
// this bounds whichever encoder is mapped to that output, at
// mediaThreads.
func outputThreadArgs() []string {
	return []string{"-threads", strconv.Itoa(mediaThreads)}
}

// libx264ThreadArgs returns the -x264-params flag that fixes
// libx264's own internal thread count directly, for Video's and
// Animation's own profiles: ffmpeg's own generic outputThreadArgs
// above reaches libx264 through its own thread_count field, but
// measured directly against a real ffmpeg encode (see
// docs/decisions.md), setting x264's own "threads" parameter directly
// as well leaves nothing about its own thread pool to that mapping.
func libx264ThreadArgs() []string {
	return []string{"-x264-params", "threads=" + strconv.Itoa(mediaThreads)}
}

// stripMetadataArgs returns the output-side arguments that strip
// everything from the output besides the streams a profile explicitly
// maps: metadata, chapters, subtitles and data streams, so nothing a
// sender wrote into those - a title, a chapter name, an embedded
// subtitle - ever reaches this package's own output file.
func stripMetadataArgs() []string {
	return []string{"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn"}
}

// maxVideoBitrateKbps and maxVideoBufsizeKbps cap libx264's own CRF
// encoding at a worst-case bitrate (-maxrate, -bufsize: the VBV
// constraints x264 enforces by spending extra quantisation rather
// than ever exceeding them), so Video's and Animation's own CRF
// setting - which has no bitrate ceiling of its own, and could reach
// far higher than usual for the highest-motion, least compressible
// input a sender could construct - can never make either profile's
// own output approach its own maxOutput cap, however adversarial the
// input. bufsize is twice maxrate, the common VBV convention: a short
// window for the encoder to average bursts over, rather than hard-
// capping every single frame's own size.
const (
	maxVideoBitrateKbps = 2500
	maxVideoBufsizeKbps = 5000
)

// videoRateControlArgs returns the VBV arguments every libx264
// profile gives ffmpeg (see maxVideoBitrateKbps). The worst case this
// cap times a profile's own duration cap could ever produce stays
// under each profile's own maxOutput: Video's own 600s *
// (2500+96) kbit/s / 8 ≈ 194.7 MB, under its 200 MiB cap with room to
// spare; Animation's own 600s * 2500 kbit/s / 8 ≈ 187.5 MB (no audio
// track) is covered by the same cap, reused rather than given a
// tighter one of its own (see animation.go).
func videoRateControlArgs() []string {
	return []string{
		"-maxrate", strconv.Itoa(maxVideoBitrateKbps) + "k",
		"-bufsize", strconv.Itoa(maxVideoBufsizeKbps) + "k",
	}
}

// fitWithinFilter returns a scale filter that fits a frame within a
// max x max box, keeping its aspect ratio, without ever upscaling a
// smaller input: both of scale's own target dimensions are capped at
// the smaller of max and the input's own corresponding dimension
// (min(max,iw) / min(max,ih)), so force_original_aspect_ratio=decrease
// can only shrink the frame to fit, never enlarge it past its own
// original size.
func fitWithinFilter(max int) string {
	return fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", max, max)
}

// outputPath returns name's own, fixed location inside the sandbox's
// output directory: the path a profile's own ffmpeg arguments must
// write to.
func outputPath(name string) string {
	return sandbox.OutputDirPath + "/" + name
}

// pcmFormatFilter is the -af filter Audio and Sound both give
// ffmpeg: keep a mono source mono, and downmix anything with more
// channels than stereo down to stereo, so the WAV header this
// package later validates only ever has to accept exactly one or
// two channels.
const pcmFormatFilter = "aformat=sample_fmts=s16:channel_layouts=mono|stereo"
