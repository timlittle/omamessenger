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

// maxSPSLength bounds an H.264 SPS NAL unit's own declared length,
// read from avcC before any of it is allocated: a real SPS is a
// handful of bytes, so a far larger declared length is already
// malformed, or an attempt to make this package read more than the
// small, fixed amount of header data it ever needs.
const maxSPSLength = 256

// maxPPSLength bounds a PPS NAL unit's own length, found in-band
// inside an avc1 sample, before this package reads any of it to check
// which sequence parameter set it names: the same reasoning as
// maxSPSLength, since a real PPS is just as small.
const maxPPSLength = 256

// maxExpGolombPrefix bounds how many leading zero bits an exp-Golomb
// field's own prefix may declare before this package gives up on it:
// every field this package actually decodes (a dimension in
// macroblocks, a small count) fits comfortably under this, and a
// crafted SPS claiming more is rejected rather than risked for an
// integer overflow computing its value.
const maxExpGolombPrefix = 16

// avcVisualSampleEntryFixedFields is the byte length of
// ISO/IEC 14496-12's VisualSampleEntry fixed fields (reserved,
// data_reference_index, width, height and the rest, up to but not
// including any child boxes such as avcC): fixed by the standard,
// never read from the file itself, so this package can safely skip
// straight past them to look for avcC.
const avcVisualSampleEntryFixedFields = 78

// allowedISOVideoCodec names every MP4/MOV sample entry type (the
// sample description box's own 4-byte format, never a sender's mime
// type or file extension) this package accepts: H.264 (avc1 only).
// avc3 is refused even though QtMultimedia can decode it: it allows a
// stream to carry (or switch) its own parameter sets in-band, inside
// sample data this package never reads, so there is no way to bound
// the dimensions FFmpeg will actually decode at. VP9 and AV1 (vp09,
// av01) are refused for the same underlying reason: their own
// configuration boxes (vpcC, av1C) do not carry a video's coded
// dimensions at all - those live only in the compressed bitstream
// itself, which this package never decodes - so a container claiming
// either codec can never be bound here and is always reported as not
// playable (see docs/decisions.md).
var allowedISOVideoCodec = map[string]bool{
	"avc1": true,
}

// allowedISOAudioCodec names every MP4/MOV audio sample entry type
// this package accepts: AAC (mp4a - the sample entry type MP4 always
// uses for AAC, never checked further against its own esds box) and
// Opus.
var allowedISOAudioCodec = map[string]bool{
	"mp4a": true,
	"Opus": true,
}

// No WebM/Matroska video codec is accepted for a video track - see
// matroskaTrackAllowed in matroska.go and docs/decisions.md for why.

// allowedMatroskaAudioCodec names every WebM/Matroska CodecID this
// package accepts for an audio track: AAC, Opus and Vorbis.
var allowedMatroskaAudioCodec = map[string]bool{
	"A_AAC":    true,
	"A_OPUS":   true,
	"A_VORBIS": true,
}
