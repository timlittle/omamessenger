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
