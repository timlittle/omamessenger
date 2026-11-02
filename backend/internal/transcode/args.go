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

// commonInputArgs returns the input-side arguments every profile
// gives ffmpeg, in order: no interactive prompts, overwrite its
// output without asking, log only errors, two decode/encode threads,
// this package's own single-allocation cap, and the one input file,
// read only as a local file (-protocol_whitelist file) so a crafted
// container cannot make ffmpeg's own demuxer reach out to anything
// else, a network address included.
func commonInputArgs() []string {
	return []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", "2",
		"-max_alloc", strconv.Itoa(maxSingleAllocBytes),
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
	}
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
