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

// mp3BitrateKbps maps an MPEG-1 Layer III frame header's own 4-bit
// bitrate index to its bitrate in kbps; index 0 ("free" bitrate) and
// 15 (reserved) are deliberately absent, so a lookup miss alone
// rejects both - a free-bitrate or VBR-without-Xing stream is not the
// simple case this package validates (see package doc).
var mp3BitrateKbps = map[byte]int{
	1: 32, 2: 40, 3: 48, 4: 56, 5: 64, 6: 80, 7: 96,
	8: 112, 9: 128, 10: 160, 11: 192, 12: 224, 13: 256, 14: 320,
}

// mp3SampleRateHz maps an MPEG-1 frame header's own 2-bit sample-rate
// index to its rate in Hz; index 3 (reserved) is absent.
var mp3SampleRateHz = map[byte]int{
	0: 44_100, 1: 48_000, 2: 32_000,
}

// mp3FramesToConfirm is how many consecutive frame headers
// validateMP3 must find, each at the offset the previous frame's own
// declared length predicts, before trusting that the file is really
// framed MP3 and not arbitrary bytes that merely happened to start
// with a sync pattern.
const mp3FramesToConfirm = 3

// mp3ScanChunkSize bounds how many bytes mp3HasForbiddenTag reads
// into memory at once while scanning a whole file for a forbidden tag
// signature, so a file many megabytes long is never loaded in full.
const mp3ScanChunkSize = 1 << 16 // 64 KiB

// mp3ForbiddenTagMarkers names every byte signature mp3HasForbiddenTag
// refuses to find anywhere in an MP3 file: the start of an ID3v2 tag
// ("ID3" - wherever it appears, at the front, appended after the
// audio, or as a second tag), an APE tag's own footer or header
// ("APETAGEX"), and a Lyrics3 tag, v1 or v2 (both open with
// "LYRICSBEGIN"). A differential security review found that
// audiosafe's own ID3v2 frame walker and FFmpeg's id3v2 demuxer parse
// a v2.4 frame's own size field differently once its high bit is set,
// letting an attached picture hide in bytes the walker treated as
// padding; rather than track every way the two could still disagree,
// an MP3 is only ever played in-window when none of these tags are
// present at all (see mp3HasForbiddenTag and docs/decisions.md).
var mp3ForbiddenTagMarkers = [][]byte{
	[]byte("ID3"),
	[]byte("APETAGEX"),
	[]byte("LYRICSBEGIN"),
}

// mp3MaxForbiddenMarkerLen is the longest byte sequence in
// mp3ForbiddenTagMarkers, the amount of overlap mp3HasForbiddenTag
// must carry from one scanned chunk into the next so a marker
// straddling a chunk boundary is never missed.
const mp3MaxForbiddenMarkerLen = len("LYRICSBEGIN")
