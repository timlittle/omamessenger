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
