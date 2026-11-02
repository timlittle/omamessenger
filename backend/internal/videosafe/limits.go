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
// declare, and is 1: a differential security review found that
// FFmpeg's own mov demuxer opens and decodes every track whose codec
// fourcc it recognizes, regardless of whether this package judged it
// or counted it toward anything, so a second "unjudged" track -
// nominally a thumbnail, or any other kind - was never actually safe
// to allow through. A playable file now declares exactly the tracks
// this package has validated: one video track, and at most one audio
// track (see maxAudioTracks); any other track of any kind refuses
// the whole file (see isoTrackAllowed and docs/decisions.md).
const maxVideoTracks = 1

// maxAudioTracks bounds how many audio tracks a single file may
// declare: at most one, the same reasoning maxVideoTracks documents.
const maxAudioTracks = 1

// maxSampleCount bounds a single track's own declared sample count
// (MP4's stsz/stz2 sample_count field), read as one 32-bit value and
// never used to allocate anything: this guards against an absurd
// declared count signalling a corrupt or hostile file, not a real
// resource cost this package itself ever pays.
const maxSampleCount = 10_000_000

// maxAVCChunkCount bounds how many chunk offsets (stco/co64 entries)
// an avc1 track's own sample layout may declare. A valid stsc table
// assigns at least one sample to every chunk a layout actually uses
// (see sampleToChunkTable's own validation), so a layout can never
// legitimately need more chunks than maxSampleCount already allows
// samples; any more is refused before avcsamples.go allocates the
// offsets table at all.
const maxAVCChunkCount = maxSampleCount

// maxAVCStscEntries bounds how many sample-to-chunk rows (stsc
// entries) an avc1 track's own sample layout may declare: each row
// covers at least one chunk holding at least one sample once
// sampleToChunkTable has validated it, so a layout can never
// legitimately need more rows than maxSampleCount allows samples
// either.
const maxAVCStscEntries = maxSampleCount

// avcSampleCancelCheckInterval bounds how many of an avc1 track's own
// samples avcSamplesSafe scans between checks of the caller's own
// context: often enough that a cancelled fetch stops within a small
// fraction of a second even at maxSampleCount, cheap enough next to
// scanning a sample's own bytes that checking every single one would
// buy nothing.
const avcSampleCancelCheckInterval = 4096

// maxBoxBudget bounds how many container boxes or EBML elements a
// single Validate call will ever read the header of, across every
// nesting level combined, so a file built from a long chain of tiny,
// otherwise well-formed boxes cannot make this package spend
// meaningfully more time on it than an ordinary file this size would.
const maxBoxBudget = 10_000

// maxCoverArtDepth bounds how many levels deep hasCoverArtAnywhere's
// own recursive walk will descend through nested container boxes
// (moov > trak > udta > meta > ilst): real MP4/MOV files never nest
// anywhere near this deep, so a file crafted with many pointless
// levels of nesting cannot grow this scan's own recursion depth
// without bound.
const maxCoverArtDepth = 16

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

// maxH264RefFrames bounds an SPS's own declared max_num_ref_frames:
// 16, the absolute ceiling every H.264 level allows (Annex A caps
// MaxDpbFrames at 16 regardless of level). FFmpeg's decoder allocates
// one reference-picture buffer per frame this field declares, at the
// coded (not cropped) size - see maxCodedFrameBudget and
// docs/decisions.md.
const maxH264RefFrames = 16

// maxCodedFrameBudget bounds an SPS's own coded pixel count (coded
// width times coded height, both already bounded to maxDimension)
// multiplied by one more than its own declared max_num_ref_frames -
// the number of same-sized frame buffers FFmpeg's decoder actually
// allocates (every reference frame, plus the one being decoded).
// Computed from maxDimension and maxH264RefFrames rather than a
// separate magic number, so it can never silently fall out of step
// with either: at the worst case both already allow (maxDimension
// square, maxH264RefFrames references), this is already the exact
// ceiling, making this an explicit, single assertion of a bound the
// two checks above already imply - not a looser one.
const maxCodedFrameBudget = maxDimension * maxDimension * (maxH264RefFrames + 1)

// h264LevelLimits is one entry of the H.264 specification's own
// Table A-1 (Annex A): the largest coded frame size a level allows,
// in macroblocks (maxFS), and the largest decoded picture buffer,
// also in macroblocks (maxDpbMbs) - together bounding how many
// reference frames a stream at that level and frame size may declare
// (floor(maxDpbMbs/frameSizeInMbs), separately capped at
// maxH264RefFrames).
type h264LevelLimits struct {
	maxFS, maxDpbMbs int64
}

// h264Levels maps every level_idc value Annex A defines to its own
// maxFS/maxDpbMbs pair (Table A-1). level_idc 9 is the alternate
// encoding some encoders use for level 1b, carrying the same limits
// this table gives level_idc 11 (1.1) rather than level 1's own
// tighter ones - treating it as the looser of the two costs nothing:
// level 1b's real limits are themselves tiny (99 macroblocks), far
// below anything this package would otherwise allow either way. A
// level_idc this table does not list is never produced by a real
// encoder and is refused outright, the same "cannot read it, cannot
// trust it" rule every other check in this package already applies.
var h264Levels = map[uint32]h264LevelLimits{
	9:  {396, 900},
	10: {99, 396},
	11: {396, 900},
	12: {396, 2376},
	13: {396, 2376},
	20: {396, 2376},
	21: {792, 4752},
	22: {1620, 8100},
	30: {1620, 8100},
	31: {3600, 18000},
	32: {5120, 20480},
	40: {8192, 32768},
	41: {8192, 32768},
	42: {8704, 34816},
	50: {22080, 110400},
	51: {36864, 184320},
	52: {36864, 184320},
	60: {139264, 696320},
	61: {139264, 696320},
	62: {139264, 696320},
}

// maxCropSlack bounds how many pixels frame cropping may remove from
// a coded dimension already within maxDimension: one macroblock's own
// width (16) for the horizontal axis, or one macroblock row's own
// height (16, doubled to 32 for field-coded pictures, which count two
// field lines per map unit) for the vertical axis. A real encoder
// crops only to correct for the coded size rounding up to a whole
// macroblock; a crafted SPS cropping away far more than that is
// declaring a coded size the cropped, container-agreeing size was
// never meant to reveal (see docs/decisions.md).
const maxCropSlack = 16

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

// allowedTopLevelBox names every box this package accepts at the
// file's own top level: ftyp (file type), moov (the movie metadata
// this package validates), mdat (sample data, never read here), the
// padding atoms real muxers write around mdat or moov (free, skip
// and wide - FFmpeg's own mov demuxer treats all three as no-ops),
// uuid (vendor extension data, also skipped unread) and meta (an
// iTunes-style cover art atom can sit bare at the top level too, see
// hasCoverArtAnywhere). A moof or mfra box - a fragmented MP4's own
// movie fragment and its index, holding sample tables this package
// never reads at all - is deliberately absent: FFmpeg's own mov
// demuxer still opens and decodes them (see docs/decisions.md), and
// any other box this package does not name here is refused the same
// way, never silently skipped over.
var allowedTopLevelBox = map[string]bool{
	"ftyp": true, "moov": true, "mdat": true,
	"free": true, "skip": true, "wide": true, "uuid": true,
	"meta": true,
}

// allowedMoovChild names every box this package accepts as a direct
// child of moov: mvhd (the movie header), trak (a track), udta and
// meta (already walked for cover art, see hasCoverArtAnywhere), iods
// (an MPEG-4 Initial Object Descriptor some encoders still write for
// backward compatibility, never read here) and the same padding
// atoms allowedTopLevelBox allows. A cmov - a compressed moov that
// FFmpeg's own mov demuxer inflates and reparses as a second,
// completely unvalidated moov - is deliberately absent, the exact
// gap a differential security review found; so is mvex, the box a
// fragmented MP4 carries its own sample tables in. Any other child
// is refused the same way.
var allowedMoovChild = map[string]bool{
	"mvhd": true, "trak": true, "udta": true, "meta": true,
	"iods": true, "free": true, "skip": true,
}

// allowedTrakChild names every box this package accepts as a direct
// child of trak: tkhd (the track header), mdia (the media
// information this package reads into), edts (an edit list - real
// encoders write one whenever a track starts with encoder delay or
// priming, AAC's own pre-roll especially) and udta/meta, the same
// cover art placement hasCoverArtAnywhere also reads directly under
// a trak. Any other child is refused.
var allowedTrakChild = map[string]bool{
	"tkhd": true, "mdia": true, "edts": true, "udta": true, "meta": true,
}

// allowedMdiaChild names every box this package accepts as a direct
// child of mdia: mdhd (the track's own duration), hdlr (its handler
// type) and minf (the media information this package reads on
// into). Any other child is refused.
var allowedMdiaChild = map[string]bool{
	"mdhd": true, "hdlr": true, "minf": true,
}

// allowedMinfChild names every box this package accepts as a direct
// child of minf: vmhd or smhd (the video or sound media header, one
// or the other depending on the track's own kind), dinf (the
// mandatory data information box every real encoder writes, never
// read here) and stbl (the sample tables this package reads into).
// Any other child is refused.
var allowedMinfChild = map[string]bool{
	"vmhd": true, "smhd": true, "dinf": true, "stbl": true,
}

// allowedStblChild names every box this package accepts as a direct
// child of stbl: the sample tables this package itself reads (stsd,
// stsz, stz2, stco, co64, stsc - see avcsamples.go) and the ones it
// never reads but real encoders still write alongside them (stts,
// stss, ctts, sgpd, sbgp). Any other child is refused.
var allowedStblChild = map[string]bool{
	"stsd": true, "stsz": true, "stz2": true, "stco": true, "co64": true, "stsc": true,
	"stts": true, "stss": true, "ctts": true, "sgpd": true, "sbgp": true,
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
