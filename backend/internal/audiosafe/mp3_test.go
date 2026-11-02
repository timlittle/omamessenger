package audiosafe_test

// mp3_test.go confirms Validate refuses every MPEG audio frame stream
// (MP3, and the related MP2/MP1 shapes a frame header's version and
// layer bits also describe) outright, never playing one in-window at
// all. A differential security review found audiosafe's own former
// frame-chain walk only ever confirmed the first mp3FramesToConfirm
// headers matched, while Qt's FFmpeg backend demuxes the whole file:
// a crafted file could open with a few valid MP3 frames, then carry
// bytes shaped as something else entirely (an MPEG-TS stream, for
// instance, carrying video) for the rest of its length, which FFmpeg
// would then decode in-process. Chasing every way the two parsers
// could still disagree is the same open-ended problem the ID3v2 frame
// walker already lost once (see docs/decisions.md); the fix taken
// instead refuses MPEG audio altogether - such a file still opens
// fine in the user's own player, the same fallback an unsupported
// codec already takes. Voice notes (Ogg/Opus, M4A/AAC) are unaffected
// and keep their own tests in ogg_test.go and isobmff_test.go.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MP3IsNeverPlayable covers every MP3 shape this package
// used to accept conditionally - a well-formed constant-bitrate
// stream with no tag at all, one with a trailing ID3v1 tag, one with
// a leading ID3v2 tag, and plain bytes that only look like the start
// of one - all now refused outright, with or without a tag of any
// kind.
func TestValidate_MP3IsNeverPlayable(t *testing.T) {
	t.Parallel()

	audio := mp3Fixture(128, 44_100, 10)

	cases := map[string][]byte{
		"well-formed constant bitrate, no tag at all": audio,
		"a trailing ID3v1 tag":                        append(append([]byte{}, audio...), id3v1Tag()...),
		"a leading ID3v2 tag":                         append(id3v2Tag(32), audio...),
		"just a sync pattern, no further frames":      {0xFF, 0xFB, 0x90, 0x00},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "fixture.mp3", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got {
				t.Errorf("Validate() = true for %q, want false: MP3 is never playable in-window", name)
			}
		})
	}
}

// FuzzValidateMP3 seeds the corpus with MP3-shaped bytes, confirming
// Validate never panics, hangs or reads past its own bounds on input
// that merely starts with an MP3 frame sync or a legacy tag marker.
func FuzzValidateMP3(f *testing.F) {
	f.Add(mp3Fixture(128, 44_100, 10))
	f.Add(append(mp3Fixture(128, 44_100, 10), id3v1Tag()...))
	f.Add(append(id3v2Tag(32), mp3Fixture(128, 44_100, 10)...))
	f.Add([]byte{0xFF, 0xFB, 0x90, 0x00})
	f.Add([]byte("ID3"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp3", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}

// id3v1Tag builds a complete, fixed 128-byte ID3v1 trailer: the "TAG"
// signature, followed by zeroed title/artist/album/year/comment/genre
// fields - a real one's shape, used only to prove a tag of this shape
// is refused the same as any other MP3, now that no MP3 ever plays
// in-window.
func id3v1Tag() []byte {
	tag := make([]byte, 128)
	copy(tag[0:3], "TAG")
	return tag
}

// id3v2Tag builds a minimal ID3v2 tag header, with bodyLen zero bytes
// following it, used only to prove a leading tag is refused the same
// as a tag-free stream.
func id3v2Tag(bodyLen int) []byte {
	hdr := make([]byte, 10)
	copy(hdr[0:3], "ID3")
	hdr[3], hdr[4] = 3, 0 // version 2.3.0
	size := []byte{
		byte((bodyLen >> 21) & 0x7F),
		byte((bodyLen >> 14) & 0x7F),
		byte((bodyLen >> 7) & 0x7F),
		byte(bodyLen & 0x7F),
	}
	copy(hdr[6:10], size)
	return append(hdr, make([]byte, bodyLen)...)
}

// mp3Fixture returns a well-formed constant-bitrate MPEG-1 Layer III
// stream: frameCount frames at bitrateKbps/sampleRateHz, each frame's
// body zero-filled past its own 4-byte header. Used only to prove
// that even a shape this package used to accept is refused now.
func mp3Fixture(bitrateKbps, sampleRateHz, frameCount int) []byte {
	var out []byte
	for i := 0; i < frameCount; i++ {
		out = append(out, mp3Frame(bitrateKbps, sampleRateHz)...)
	}
	return out
}

// mp3Frame builds one MPEG-1 Layer III frame header plus a zero-filled
// body of exactly its own declared length (no padding bit set).
func mp3Frame(bitrateKbps, sampleRateHz int) []byte {
	bitrateIndex := mp3IndexForBitrate(bitrateKbps)
	sampleRateIndex := mp3IndexForSampleRate(sampleRateHz)

	hdr := make([]byte, 4)
	hdr[0] = 0xFF
	hdr[1] = 0xFB // sync (3 more bits) + MPEG-1 (11) + Layer III (01)
	hdr[2] = bitrateIndex<<4 | sampleRateIndex<<2
	hdr[3] = 0x00

	frameLen := 144 * bitrateKbps * 1000 / sampleRateHz
	body := make([]byte, frameLen-4)
	return append(hdr, body...)
}

// mp3IndexForBitrate reverses the MPEG-1 Layer III bitrate table for
// the handful of values this file's fixtures use.
func mp3IndexForBitrate(kbps int) byte {
	table := map[int]byte{32: 1, 40: 2, 48: 3, 56: 4, 64: 5, 80: 6, 96: 7, 112: 8, 128: 9, 160: 10, 192: 11, 224: 12, 256: 13, 320: 14}
	return table[kbps]
}

// mp3IndexForSampleRate reverses the MPEG-1 sample-rate table for the
// handful of values this file's fixtures use.
func mp3IndexForSampleRate(hz int) byte {
	table := map[int]byte{44_100: 0, 48_000: 1, 32_000: 2}
	return table[hz]
}
