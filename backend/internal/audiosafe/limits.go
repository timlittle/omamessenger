package audiosafe

import "time"

// limits.go bounds every check Validate runs, the same discipline
// videosafe's own limits.go applies to video: the shell must never be
// asked to decode an audio file whose declared length or structure
// could make QtMultimedia itself do unbounded work, and this package
// must never do more than a small, fixed amount of work itself
// reading a crafted header.

// maxDuration bounds an audio file's own declared or estimated
// length. 30 minutes comfortably covers a voice note or a shared
// song; nothing this UI sends or receives is meant to be longer.
const maxDuration = 30 * time.Minute

// maxOggPages bounds how many Ogg page headers validateOgg will ever
// read, across one whole Validate call, so a file built from a long
// chain of tiny, otherwise well-formed pages cannot make this package
// spend meaningfully more time on it than an ordinary file this size
// would. 30 minutes of Opus at its smallest common frame size (2.5ms)
// is well under 1,000,000 pages' worth of frames (Opus typically
// carries several frames per page); this leaves generous headroom.
const maxOggPages = 1_000_000

// maxBoxBudget bounds how many ISO BMFF boxes a single Validate call
// will ever read the header of, across every nesting level combined,
// mirroring videosafe's own box budget.
const maxBoxBudget = 10_000

// maxCoverArtDepth bounds how many levels deep hasCoverArtAnywhere's
// own recursive walk will descend through nested container boxes
// (moov > trak > udta > meta > ilst): real MP4/M4A files never nest
// anywhere near this deep, so a file crafted with many pointless
// levels of nesting cannot grow this scan's own recursion depth
// without bound.
const maxCoverArtDepth = 16

// opusGranuleRate is Opus's own fixed sample rate for its granule
// position timestamps (RFC 7845): every Opus stream's granule
// position counts 48kHz samples regardless of the stream's actual
// encoded sample rate, so duration is always granule/48000 seconds.
const opusGranuleRate = 48_000

// allowedISOAudioCodec names every MP4/M4A sample entry type this
// package accepts for the file's one audio track: AAC (mp4a, M4A's
// usual codec) and Opus, the same pair videosafe's own
// allowedISOAudioCodec allows for a video's audio track.
var allowedISOAudioCodec = map[string]bool{
	"mp4a": true,
	"Opus": true,
}

// allowedTopLevelBox names every box this package accepts at the
// file's own top level, the same set videosafe's own
// allowedTopLevelBox documents and for the same reasons: ftyp, moov,
// mdat, the free/skip/wide padding atoms, uuid and a bare top-level
// meta (an iTunes-style cover art atom can sit there too, see
// hasCoverArtAnywhere). A moof or mfra box is deliberately absent, and
// any other box this package does not name here is refused the same
// way.
var allowedTopLevelBox = map[string]bool{
	"ftyp": true, "moov": true, "mdat": true,
	"free": true, "skip": true, "wide": true, "uuid": true,
	"meta": true,
}

// allowedMoovChild names every box this package accepts as a direct
// child of moov, the same set and reasoning videosafe's own
// allowedMoovChild documents: mvhd, trak, udta, meta, iods, free and
// skip. A cmov - a compressed moov FFmpeg's own mov demuxer inflates
// and reparses as a second, completely unvalidated moov - is
// deliberately absent, the exact gap a differential security review
// found; so is mvex, a fragmented MP4's own sample tables. Any other
// child is refused.
var allowedMoovChild = map[string]bool{
	"mvhd": true, "trak": true, "udta": true, "meta": true,
	"iods": true, "free": true, "skip": true,
}

// allowedTrakChild names every box this package accepts as a direct
// child of trak, the same set videosafe's own allowedTrakChild
// documents: tkhd, mdia, edts and udta/meta. Any other child is
// refused.
var allowedTrakChild = map[string]bool{
	"tkhd": true, "mdia": true, "edts": true, "udta": true, "meta": true,
}

// allowedMdiaChild names every box this package accepts as a direct
// child of mdia: mdhd, hdlr and minf. Any other child is refused.
var allowedMdiaChild = map[string]bool{
	"mdhd": true, "hdlr": true, "minf": true,
}

// allowedMinfChild names every box this package accepts as a direct
// child of minf: vmhd or smhd, dinf and stbl. Any other child is
// refused.
var allowedMinfChild = map[string]bool{
	"vmhd": true, "smhd": true, "dinf": true, "stbl": true,
}

// allowedStblChild names every box this package accepts as a direct
// child of stbl: stsd (the sample entry this package reads) and the
// boxes it never reads but real encoders still write alongside it
// (stts, stsc, stsz, stz2, stco, co64, stss, ctts, sgpd, sbgp - the
// same set videosafe's own allowedStblChild documents). Any other
// child is refused.
var allowedStblChild = map[string]bool{
	"stsd": true, "stsz": true, "stz2": true, "stco": true, "co64": true, "stsc": true,
	"stts": true, "stss": true, "ctts": true, "sgpd": true, "sbgp": true,
}
