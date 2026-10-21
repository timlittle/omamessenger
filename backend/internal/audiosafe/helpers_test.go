package audiosafe_test

// helpers_test.go builds minimal, well-formed Ogg/Opus, MP4/M4A and
// MP3 byte streams for the table and fuzz tests in this package to
// start from and mutate, the same reasoning videosafe's own
// helpers_test.go documents: each builder takes only the handful of
// fields this package actually reads, so a test can construct
// exactly the file it wants to check without a real-world sample
// file checked into the repository.

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeFixture writes data to a file in t.TempDir() and returns its path.
func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// oggOpusParams names everything oggOpusFixture varies between test cases.
type oggOpusParams struct {
	secondSerial bool   // true makes a later page claim a different bitstream serial
	skipOpusHead bool   // true makes the first page's payload not start with "OpusHead"
	granule      uint64 // the final page's granule position; 0 when zero-value
	pageCount    int    // how many pages to emit; defaults to 1
}

// oggOpusFixture returns a minimal, well-formed single-bitstream Ogg
// file whose first page's payload opens with "OpusHead", unless
// skipOpusHead asks for something else; its last page carries
// granule as its granule position.
func oggOpusFixture(p oggOpusParams) []byte {
	if p.pageCount == 0 {
		p.pageCount = 1
	}

	serial := uint32(1)
	var out []byte
	for i := 0; i < p.pageCount; i++ {
		headerType := byte(0)
		if i == 0 {
			headerType |= 0x02 // BOS
		}
		if i == p.pageCount-1 {
			headerType |= 0x04 // EOS
		}

		pageSerial := serial
		if p.secondSerial && i == p.pageCount-1 {
			pageSerial = serial + 1
		}

		payload := []byte("OpusHead further opus header bytes here")
		if i == 0 && p.skipOpusHead {
			payload = []byte("not an opus packet at all, just filler")
		}
		if i > 0 {
			payload = []byte("opus audio data frame")
		}

		granule := uint64(0)
		if i == p.pageCount-1 {
			granule = p.granule
		}

		out = append(out, oggPage(headerType, granule, pageSerial, uint32(i), payload)...)
	}
	return out
}

// oggPage builds one Ogg page: a 27-byte header, a one-byte segment
// table naming payload's own length (bounded to 255 here, enough for
// every fixture this package builds), and payload itself.
func oggPage(headerType byte, granule uint64, serial, sequence uint32, payload []byte) []byte {
	hdr := make([]byte, 27)
	copy(hdr[0:4], "OggS")
	hdr[4] = 0 // version
	hdr[5] = headerType
	binary.LittleEndian.PutUint64(hdr[6:14], granule)
	binary.LittleEndian.PutUint32(hdr[14:18], serial)
	binary.LittleEndian.PutUint32(hdr[18:22], sequence)
	// hdr[22:26] CRC, left zero: this package never checks it
	hdr[26] = 1 // one segment table entry

	hdr = append(hdr, byte(len(payload)))
	return append(hdr, payload...)
}

// mp4AudioParams names everything mp4AudioFixture varies.
type mp4AudioParams struct {
	audioCodec   string // 4 bytes; "mp4a" when zero
	videoCodec   string // adding a video trak when set, to prove it is rejected
	timescale    uint32 // defaults to 1000 when zero
	durationUnit uint32
	secondAudio  bool // adds a second audio trak, to prove multiple is rejected
}

// mp4AudioFixture returns a minimal, well-formed MP4/M4A file: an
// moov box with an mvhd and one audio trak (and, with videoCodec
// set, one video trak too).
func mp4AudioFixture(p mp4AudioParams) []byte {
	if p.audioCodec == "" {
		p.audioCodec = "mp4a"
	}
	if p.timescale == 0 {
		p.timescale = 1000
	}

	mvhd := mp4Box("mvhd", mp4MediaHeader(p.timescale, p.durationUnit))
	audioTrak := mp4AudioTrak(p.audioCodec, p.timescale, p.durationUnit)

	moovBody := append(append([]byte{}, mvhd...), audioTrak...)
	if p.secondAudio {
		moovBody = append(moovBody, mp4AudioTrak(p.audioCodec, p.timescale, p.durationUnit)...)
	}
	if p.videoCodec != "" {
		moovBody = append(moovBody, mp4VideoTrak(p.videoCodec, p.timescale, p.durationUnit)...)
	}

	out := append([]byte{}, mp4Box("ftyp", []byte("isom0000isomiso2mp41"))...)
	return append(out, mp4Box("moov", moovBody)...)
}

// mp4AudioTrak builds one audio trak box: mdia > hdlr ("soun"), mdia
// > mdhd (duration), mdia > minf > stbl > stsd (the sample entry
// carrying codec).
func mp4AudioTrak(codec string, timescale, durationUnit uint32) []byte {
	return mp4Trak("soun", codec, timescale, durationUnit)
}

// mp4VideoTrak builds one video trak box with handler "vide", used
// only to prove a file carrying a video track is always rejected
// here, regardless of its own codec or dimensions.
func mp4VideoTrak(codec string, timescale, durationUnit uint32) []byte {
	return mp4Trak("vide", codec, timescale, durationUnit)
}

// mp4Trak builds one trak box with the given handler and codec.
func mp4Trak(handler, codec string, timescale, durationUnit uint32) []byte {
	mdhd := mp4Box("mdhd", mp4MediaHeader(timescale, durationUnit))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte(handler)...))
	stsd := mp4Box("stsd", mp4SampleDescription(codec))
	stbl := mp4Box("stbl", stsd)
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	return mp4Box("trak", mdia)
}

// mp4MediaHeader builds an mvhd/mdhd body (version 0): 4 bytes of
// version/flags, two 4-byte timestamps, the timescale, then the
// duration, the only fields either box's own reader ever looks at.
func mp4MediaHeader(timescale, duration uint32) []byte {
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[12:16], timescale)
	binary.BigEndian.PutUint32(buf[16:20], duration)
	return buf
}

// mp4MediaHeaderV1 builds an mvhd/mdhd body with version 1 (64-bit
// timestamps and duration), the less common of the two layouts
// boxDuration reads.
func mp4MediaHeaderV1(timescale uint32, duration uint64) []byte {
	buf := make([]byte, 32)
	buf[0] = 1 // version
	binary.BigEndian.PutUint32(buf[20:24], timescale)
	binary.BigEndian.PutUint64(buf[24:32], duration)
	return buf
}

// mp4SampleDescription builds an stsd body holding exactly one sample
// entry: version/flags and entry_count (8 bytes), then the entry's
// own size+type header (8 bytes, type set to codec) padded out to a
// plausible audio sample entry size.
func mp4SampleDescription(codec string) []byte {
	buf := make([]byte, 8+20)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], 20)
	copy(buf[12:16], codec)
	return buf
}

// mp4Box wraps body in a box with the given four-character type and
// a plain 32-bit size header.
func mp4Box(boxType string, body []byte) []byte {
	buf := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(buf)))
	copy(buf[4:8], boxType)
	copy(buf[8:], body)
	return buf
}

// mp3Fixture returns a well-formed constant-bitrate MPEG-1 Layer III
// stream: frameCount frames at bitrateKbps/sampleRateHz, each frame's
// body zero-filled past its own 4-byte header, which is all this
// package ever reads.
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

// mp3IndexForBitrate reverses mp3BitrateKbps's own table for the
// handful of values this package's fixtures use.
func mp3IndexForBitrate(kbps int) byte {
	table := map[int]byte{32: 1, 40: 2, 48: 3, 56: 4, 64: 5, 80: 6, 96: 7, 112: 8, 128: 9, 160: 10, 192: 11, 224: 12, 256: 13, 320: 14}
	return table[kbps]
}

// mp3IndexForSampleRate reverses mp3SampleRateHz's own table.
func mp3IndexForSampleRate(hz int) byte {
	table := map[int]byte{44_100: 0, 48_000: 1, 32_000: 2}
	return table[hz]
}

// id3v2Tag builds a minimal ID3v2 tag header, with bodyLen zero
// bytes following it, that this package's own mp3StartOffset must
// skip over before reading the first real frame.
func id3v2Tag(bodyLen int) []byte {
	hdr := make([]byte, 10)
	copy(hdr[0:3], "ID3")
	hdr[3], hdr[4] = 3, 0 // version 2.3.0
	size := synchsafeEncode(bodyLen)
	copy(hdr[6:10], size)
	return append(hdr, make([]byte, bodyLen)...)
}

// synchsafeEncode encodes n as a 4-byte ID3v2 synchsafe size field.
func synchsafeEncode(n int) []byte {
	return []byte{
		byte((n >> 21) & 0x7F),
		byte((n >> 14) & 0x7F),
		byte((n >> 7) & 0x7F),
		byte(n & 0x7F),
	}
}
