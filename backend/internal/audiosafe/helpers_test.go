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
	secondSerial      bool     // true makes a later page claim a different bitstream serial
	skipOpusHead      bool     // true makes the first page's payload not start with "OpusHead"
	granule           uint64   // the final page's granule position; 0 when zero-value
	pageCount         int      // how many pages to emit; defaults to 2 (the ID and comment headers, the minimum a real Opus stream always has)
	commentComments   []string // comments the second page's comment header packet carries; none when nil
	invalidCommentTag bool     // true makes the second page's payload something other than a parseable "OpusTags" packet
}

// oggOpusFixture returns a minimal, well-formed single-bitstream Ogg
// file whose first page's payload opens with "OpusHead" (unless
// skipOpusHead asks for something else) and whose second page carries
// a parseable "OpusTags" comment header packet (unless
// invalidCommentTag asks for something else), the two packets a real
// Opus stream always carries before any audio data; its last page
// carries granule as its granule position.
func oggOpusFixture(p oggOpusParams) []byte {
	if p.pageCount == 0 {
		p.pageCount = 2
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

		granule := uint64(0)
		if i == p.pageCount-1 {
			granule = p.granule
		}

		out = append(out, oggPage(headerType, granule, pageSerial, uint32(i), oggOpusPagePayload(p, i))...)
	}
	return out
}

// oggOpusPagePayload returns oggOpusFixture's own page i's payload:
// the ID header for page 0, a comment header packet for page 1, and a
// filler audio frame for every page after that.
func oggOpusPagePayload(p oggOpusParams, i int) []byte {
	switch {
	case i == 0 && p.skipOpusHead:
		return []byte("not an opus packet at all, just filler")
	case i == 0:
		return []byte("OpusHead further opus header bytes here")
	case i == 1 && p.invalidCommentTag:
		return []byte("not an opus tags packet at all, just filler")
	case i == 1:
		return opusTagsPacket(p.commentComments...)
	default:
		return []byte("opus audio data frame")
	}
}

// opusTagsPacket builds a minimal, well-formed Opus comment header
// packet ("OpusTags"): an empty vendor string and the given comments,
// each already in "KEY=VALUE" form.
func opusTagsPacket(comments ...string) []byte {
	buf := append([]byte{}, []byte("OpusTags")...)
	buf = append(buf, 0, 0, 0, 0) // vendor_string_length = 0: no vendor string

	countBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(countBuf, uint32(len(comments)))
	buf = append(buf, countBuf...)

	for _, comment := range comments {
		lenBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBuf, uint32(len(comment)))
		buf = append(buf, lenBuf...)
		buf = append(buf, []byte(comment)...)
	}

	return buf
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
	secondAudio  bool   // adds a second audio trak, to prove multiple is rejected
	stsdEntries  uint32 // the audio trak's own stsd entry_count; 1 when zero
	coverArt     bool   // true adds an iTunes-style moov > udta > meta > ilst > covr atom
	// coverArtPlacement adds a cover art atom at one of the other
	// places FFmpeg's own mov demuxer also reads one from, which the
	// narrow moov > udta > meta > ilst > covr check alone used to miss
	// (see isobmff.go's own hasCoverArtAnywhere and docs/decisions.md):
	// "moovMeta" (meta directly under moov, no udta wrapper), "secondUdta"
	// (an empty first udta, then a second one carrying the real atom),
	// "malformedDecoy" (a malformed box inside udta, before the real
	// meta), or "topLevel" (a bare meta at the file's own top level,
	// outside moov entirely).
	coverArtPlacement string
}

// mp4AudioFixture returns a minimal, well-formed MP4/M4A file: an
// moov box with an mvhd and one audio trak (and, with videoCodec
// set, one video trak too). With coverArt set, the moov box also
// carries an iTunes-style cover art atom, the shape this package must
// refuse (see isobmff.go's own hasCoverArtAtom).
func mp4AudioFixture(p mp4AudioParams) []byte {
	if p.audioCodec == "" {
		p.audioCodec = "mp4a"
	}
	if p.timescale == 0 {
		p.timescale = 1000
	}
	if p.stsdEntries == 0 {
		p.stsdEntries = 1
	}

	mvhd := mp4Box("mvhd", mp4MediaHeader(p.timescale, p.durationUnit))
	audioTrak := mp4AudioTrakWithEntries(p.audioCodec, p.timescale, p.durationUnit, p.stsdEntries)

	moovBody := append(append([]byte{}, mvhd...), audioTrak...)
	if p.secondAudio {
		moovBody = append(moovBody, mp4AudioTrak(p.audioCodec, p.timescale, p.durationUnit)...)
	}
	if p.videoCodec != "" {
		moovBody = append(moovBody, mp4VideoTrak(p.videoCodec, p.timescale, p.durationUnit)...)
	}
	if p.coverArt {
		moovBody = append(moovBody, mp4CoverArtUdta()...)
	}
	switch p.coverArtPlacement {
	case "moovMeta":
		moovBody = append(moovBody, mp4CoverArtMeta()...)
	case "secondUdta":
		moovBody = append(moovBody, mp4CoverArtUnderSecondUdta()...)
	case "malformedDecoy":
		moovBody = append(moovBody, mp4CoverArtBehindMalformedBox()...)
	}

	out := append([]byte{}, mp4Box("ftyp", []byte("isom0000isomiso2mp41"))...)
	out = append(out, mp4Box("moov", moovBody)...)
	if p.coverArtPlacement == "topLevel" {
		out = append(out, mp4CoverArtMeta()...)
	}
	return out
}

// mp4CoverArtMeta builds a minimal iTunes-style meta > ilst > covr
// atom on its own, carrying a handful of fake image bytes as covr's
// own body: the exact shape FFmpeg's own mov demuxer reads as an
// attached picture (see isobmff.go's own hasCoverArtAnywhere and
// docs/decisions.md), wherever a caller places it.
func mp4CoverArtMeta() []byte {
	covr := mp4Box("covr", []byte("fake cover art image bytes"))
	ilst := mp4Box("ilst", covr)
	return mp4Box("meta", append(make([]byte, 4), ilst...)) // 4 bytes of version/flags
}

// mp4CoverArtUdta builds a minimal iTunes-style
// udta > meta > ilst > covr atom, the common placement.
func mp4CoverArtUdta() []byte {
	return mp4Box("udta", mp4CoverArtMeta())
}

// mp4CoverArtUnderSecondUdta builds two sibling udta boxes: an empty
// first one, then a second carrying the real cover art atom, covering
// the gap the earlier, narrower check had by only ever looking at the
// first udta it found.
func mp4CoverArtUnderSecondUdta() []byte {
	empty := mp4Box("udta", []byte{})
	return append(empty, mp4CoverArtUdta()...)
}

// mp4CoverArtBehindMalformedBox builds a udta box whose own body opens
// with a box header declaring a size past its container, before the
// real meta > ilst > covr atom: the earlier, narrower check's own
// firstChild stopped at the first box it could not parse and reported
// "not found" rather than refusing the file, so a decoy like this
// placed ahead of the real atom hid it entirely.
func mp4CoverArtBehindMalformedBox() []byte {
	malformed := make([]byte, 8)
	binary.BigEndian.PutUint32(malformed[0:4], 999_999)
	copy(malformed[4:8], "xxxx")
	body := append(append([]byte{}, malformed...), mp4CoverArtMeta()...)
	return mp4Box("udta", body)
}

// mp4AudioTrak builds one audio trak box: mdia > hdlr ("soun"), mdia
// > mdhd (duration), mdia > minf > stbl > stsd (the sample entry
// carrying codec).
func mp4AudioTrak(codec string, timescale, durationUnit uint32) []byte {
	return mp4Trak("soun", codec, timescale, durationUnit)
}

// mp4AudioTrakWithEntries builds one audio trak box exactly like
// mp4AudioTrak, except its stsd box declares entryCount sample
// entries - each a full copy of the same codec entry - instead of
// always exactly one: for a test that checks a track is refused
// outright once entry_count is anything but 1, the same gap
// videosafe's own second-stsd-entry test covers (see
// docs/decisions.md).
func mp4AudioTrakWithEntries(codec string, timescale, durationUnit, entryCount uint32) []byte {
	mdhd := mp4Box("mdhd", mp4MediaHeader(timescale, durationUnit))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	stsd := mp4Box("stsd", mp4SampleDescriptionMultiEntry(codec, entryCount))
	stbl := mp4Box("stbl", stsd)
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	return mp4Box("trak", mdia)
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
	return mp4SampleDescriptionMultiEntry(codec, 1)
}

// mp4SampleDescriptionMultiEntry builds an stsd body like
// mp4SampleDescription, but declaring entryCount sample entries - each
// an identical copy of the same codec entry - instead of always
// exactly one.
func mp4SampleDescriptionMultiEntry(codec string, entryCount uint32) []byte {
	entry := make([]byte, 20)
	binary.BigEndian.PutUint32(entry[0:4], 20)
	copy(entry[4:8], codec)

	buf := make([]byte, 8, 8+20*int(entryCount))
	binary.BigEndian.PutUint32(buf[4:8], entryCount)
	for i := uint32(0); i < entryCount; i++ {
		buf = append(buf, entry...)
	}
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
// bytes following it (read as padding, never a real frame), that
// this package's own id3v2Scan must walk over before reading the
// first real MP3 frame.
func id3v2Tag(bodyLen int) []byte {
	hdr := make([]byte, 10)
	copy(hdr[0:3], "ID3")
	hdr[3], hdr[4] = 3, 0 // version 2.3.0
	size := synchsafeEncode(bodyLen)
	copy(hdr[6:10], size)
	return append(hdr, make([]byte, bodyLen)...)
}

// id3v2TagWithFrames builds a complete ID3v2 tag of the given version
// (2, 3 or 4), its own declared size computed from the frames it
// carries, with no padding beyond their own bytes.
func id3v2TagWithFrames(version byte, frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}

	hdr := make([]byte, 10)
	copy(hdr[0:3], "ID3")
	hdr[3], hdr[4] = version, 0
	copy(hdr[6:10], synchsafeEncode(len(body)))
	return append(hdr, body...)
}

// id3v2Frame builds one ID3v2 frame: frameID (3 bytes for version 2,
// 4 bytes for version 3 or 4), its own declared size (plain
// big-endian for version 2 or 3, synchsafe for version 4) and body.
// Version 3 and 4 frames get a 2-byte all-zero flags field; version 2
// frames have none.
func id3v2Frame(version byte, frameID string, body []byte) []byte {
	idLen := 3
	if version >= 3 {
		idLen = 4
	}
	id := make([]byte, idLen)
	copy(id, frameID)

	var sizeField []byte
	switch version {
	case 4:
		sizeField = synchsafeEncode(len(body))
	case 3:
		sizeField = []byte{
			byte(len(body) >> 24), byte(len(body) >> 16),
			byte(len(body) >> 8), byte(len(body)),
		}
	default: // version 2
		sizeField = []byte{byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	}

	frame := append(append([]byte{}, id...), sizeField...)
	if version >= 3 {
		frame = append(frame, 0, 0) // frame flags
	}
	return append(frame, body...)
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
