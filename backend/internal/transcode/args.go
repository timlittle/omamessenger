package transcode

// args.go holds the ffmpeg argument pieces every profile shares.

import (
	"fmt"
	"strconv"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// maxSingleAllocBytes bounds any one avutil allocation (-max_alloc),
// well under the sandbox's address-space limit, so a crafted file
// trying to force a huge allocation hits this first.
const maxSingleAllocBytes = 512 << 20

// mediaThreads bounds how many threads any one stage of ffmpeg's
// pipeline (decode, filter, encode) may use, since the sandbox caps
// CPU time and priority but not thread count (see docs/decisions.md).
const mediaThreads = 2

// commonInputArgs returns the input-side arguments every profile
// gives ffmpeg: no prompts, overwrite output, errors-only logging,
// mediaThreads decode threads, the alloc cap, and the input file
// restricted to the local-file protocol so a crafted container
// cannot make the demuxer reach a network address.
func commonInputArgs() []string {
	return []string{
		"-nostdin", "-y", "-v", "error",
		"-threads", strconv.Itoa(mediaThreads),
		"-max_alloc", strconv.Itoa(maxSingleAllocBytes),
		"-protocol_whitelist", "file",
		"-i", sandbox.InputPath,
	}
}

// filterThreadArgs returns the -filter_threads argument, bounding a
// profile's -vf/-af filter graph to mediaThreads.
func filterThreadArgs() []string {
	return []string{"-filter_threads", strconv.Itoa(mediaThreads)}
}

// outputThreadArgs returns the -threads argument on the output side,
// bounding the encoder to mediaThreads.
func outputThreadArgs() []string {
	return []string{"-threads", strconv.Itoa(mediaThreads)}
}

// libx264ThreadArgs returns the -x264-params flag that fixes
// libx264's internal thread count directly, since outputThreadArgs
// alone does not reliably bound it (confirmed against a real encode).
func libx264ThreadArgs() []string {
	return []string{"-x264-params", "threads=" + strconv.Itoa(mediaThreads)}
}

// stripMetadataArgs strips metadata, chapters, subtitles and data
// streams, so nothing a sender wrote into those reaches the output.
func stripMetadataArgs() []string {
	return []string{"-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn"}
}

// maxVideoBitrateKbps and maxVideoBufsizeKbps cap libx264's CRF
// encoding at a worst-case bitrate (VBV -maxrate/-bufsize), so a
// highly adversarial input cannot push output size near maxOutput.
// bufsize is twice maxrate, the usual VBV convention.
const (
	maxVideoBitrateKbps = 2500
	maxVideoBufsizeKbps = 5000
)

// videoRateControlArgs returns the VBV arguments every libx264
// profile gives ffmpeg. Worst case (duration cap * bitrate cap)
// stays under each profile's own maxOutput.
func videoRateControlArgs() []string {
	return []string{
		"-maxrate", strconv.Itoa(maxVideoBitrateKbps) + "k",
		"-bufsize", strconv.Itoa(maxVideoBufsizeKbps) + "k",
	}
}

// fitWithinFilter returns a scale filter that fits a frame within a
// max x max box, keeping aspect ratio, without ever upscaling.
func fitWithinFilter(max int) string {
	return fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", max, max)
}

// outputPath returns name's fixed location inside the sandbox's
// output directory.
func outputPath(name string) string {
	return sandbox.OutputDirPath + "/" + name
}

// pcmFormatFilter keeps a mono source mono and downmixes anything
// wider to stereo, so the WAV header only ever needs to accept one
// or two channels.
const pcmFormatFilter = "aformat=sample_fmts=s16:channel_layouts=mono|stereo"
