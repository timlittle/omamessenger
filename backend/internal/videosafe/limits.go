package videosafe

import "time"

// limits.go bounds every check Validate runs: the shell must never be
// asked to decode a video, "GIF" or not, whose declared size, length
// or track count could make QtMultimedia itself do unbounded work,
// and this package must never do more than a small, fixed amount of
// work itself reading a crafted header.

// maxDimension bounds a video track's own declared width and height,
// each checked independently, in pixels.
const maxDimension = 1920

// maxDuration bounds a video's own declared length.
const maxDuration = 10 * time.Minute

// maxVideoTracks bounds how many video tracks a single file may
// declare. A plain video or a "GIF" video never has more than one;
// a small allowance covers an unusual file with, say, a thumbnail
// track without opening the door to an unbounded list.
const maxVideoTracks = 3

// maxSampleCount bounds a single track's own declared sample count
// (MP4's stsz/stz2 sample_count field), read as one 32-bit value and
// never used to allocate anything: this guards against an absurd
// declared count signalling a corrupt or hostile file, not a real
// resource cost this package itself ever pays.
const maxSampleCount = 10_000_000

// maxBoxBudget bounds how many container boxes or EBML elements a
// single Validate call will ever read the header of, across every
// nesting level combined, so a file built from a long chain of tiny,
// otherwise well-formed boxes cannot make this package spend
// meaningfully more time on it than an ordinary file this size would.
const maxBoxBudget = 10_000

// maxStringElement bounds an EBML string element's declared size
// (used only for a track's own CodecID, always a handful of ASCII
// characters) before this package reads it into memory at all, so a
// crafted size can never make it allocate an arbitrarily large buffer.
const maxStringElement = 256

// allowedISOVideoCodec names every MP4/MOV sample entry type (the
// sample description box's own 4-byte format, never a sender's mime
// type or file extension) this package accepts: H.264 (avc1, the
// common case, and avc3, which carries its parameter sets in-band
// rather than in the sample entry itself), VP9 and AV1.
var allowedISOVideoCodec = map[string]bool{
	"avc1": true,
	"avc3": true,
	"vp09": true,
	"av01": true,
}

// allowedISOAudioCodec names every MP4/MOV audio sample entry type
// this package accepts: AAC (mp4a - the sample entry type MP4 always
// uses for AAC, never checked further against its own esds box) and
// Opus.
var allowedISOAudioCodec = map[string]bool{
	"mp4a": true,
	"Opus": true,
}

// allowedMatroskaVideoCodec names every WebM/Matroska CodecID this
// package accepts for a video track: VP8, VP9 and AV1.
var allowedMatroskaVideoCodec = map[string]bool{
	"V_VP8": true,
	"V_VP9": true,
	"V_AV1": true,
}

// allowedMatroskaAudioCodec names every WebM/Matroska CodecID this
// package accepts for an audio track: AAC, Opus and Vorbis.
var allowedMatroskaAudioCodec = map[string]bool{
	"A_AAC":    true,
	"A_OPUS":   true,
	"A_VORBIS": true,
}
