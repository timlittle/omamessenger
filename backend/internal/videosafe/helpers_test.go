package videosafe_test

// helpers_test.go builds minimal, well-formed MP4/MOV and
// WebM/Matroska byte streams for the table and fuzz tests in
// isobmff_test.go and matroska_test.go to start from and mutate: each
// builder takes the handful of fields this package actually reads
// (codec, dimensions, duration, sample count) so a test can construct
// exactly the file it wants to check, rather than depending on a
// real-world sample file checked into the repository.

import (
	"encoding/binary"
	"math"
)

// The EBML element IDs and TrackType values webmFixture's own helpers
// below build with. These are Matroska's own fixed values, written
// out again here rather than imported from matroska.go: a fixture
// built from literal, independently-written byte values is a more
// honest check of the real parser than one built from its own
// constants.
const (
	testEBMLHeaderID    = 0x1A45DFA3
	testIDSegment       = 0x18538067
	testIDInfo          = 0x1549A966
	testIDTimecodeScale = 0x2AD7B1
	testIDDuration      = 0x4489
	testIDTracks        = 0x1654AE6B
	testIDTrackEntry    = 0xAE
	testIDTrackType     = 0x83
	testIDCodecID       = 0x86
	testIDVideo         = 0xE0
	testIDPixelWidth    = 0xB0
	testIDPixelHeight   = 0xBA
	testTrackTypeVideo  = 1
	testTrackTypeAudio  = 2
)

// mp4Params names everything mp4Fixture varies between test cases.
type mp4Params struct {
	videoCodec   string // 4 bytes; "avc1" when zero
	audioCodec   string // 4 bytes, or "" for no audio track at all
	width        uint16
	height       uint16
	timescale    uint32 // defaults to 1000 when zero
	durationUnit uint32 // duration in timescale units; callers set this explicitly, including to 0
	sampleCount  uint32
}

// mp4Fixture returns a minimal, well-formed MP4 file: an moov box
// with an mvhd and one video trak (and, with an audioCodec set, one
// audio trak), each carrying just enough of a real file's own box
// structure for this package's parser to read.
func mp4Fixture(p mp4Params) []byte {
	if p.videoCodec == "" {
		p.videoCodec = "avc1"
	}
	if p.timescale == 0 {
		p.timescale = 1000
	}

	mvhd := mp4Box("mvhd", mp4MediaHeader(p.timescale, p.durationUnit))
	videoTrak := mp4Trak(trakParams{handler: "vide", codec: p.videoCodec, width: p.width, height: p.height, timescale: p.timescale, durationUnit: p.durationUnit, sampleCount: p.sampleCount})

	moovBody := append(append([]byte{}, mvhd...), videoTrak...)
	if p.audioCodec != "" {
		moovBody = append(moovBody, mp4Trak(trakParams{handler: "soun", codec: p.audioCodec, timescale: p.timescale, durationUnit: p.durationUnit, sampleCount: p.sampleCount})...)
	}

	out := append([]byte{}, mp4Box("ftyp", []byte("isom0000isomiso2mp41"))...)
	return append(out, mp4Box("moov", moovBody)...)
}

// trakParams names everything mp4Trak varies between the tracks it builds.
type trakParams struct {
	handler      string // "vide" or "soun"
	codec        string
	width        uint16
	height       uint16
	timescale    uint32
	durationUnit uint32
	sampleCount  uint32
}

// mp4Trak builds one trak box: mdia > hdlr (handler), mdia > mdhd
// (duration), mdia > minf > stbl > stsd (sample entry, carrying the
// codec and, for a video track, its width and height) and stbl >
// stsz (sample count).
func mp4Trak(p trakParams) []byte {
	mdhd := mp4Box("mdhd", mp4MediaHeader(p.timescale, p.durationUnit))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte(p.handler)...))
	stsd := mp4Box("stsd", mp4SampleDescription(p.codec, p.width, p.height))
	stsz := mp4Box("stsz", mp4SampleSize(p.sampleCount))
	stbl := mp4Box("stbl", append(append([]byte{}, stsd...), stsz...))
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

// mp4SampleDescription builds an stsd body holding exactly one
// sample entry: version/flags and entry_count (8 bytes), the entry's
// own size+type header (8 bytes, type set to codec), then enough of
// VisualSampleEntry's fixed fields to put width and height at their
// real offset - this package's own sampleEntryDims reads exactly this
// layout.
func mp4SampleDescription(codec string, width, height uint16) []byte {
	buf := make([]byte, 8+44)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], 44)
	copy(buf[12:16], codec)
	binary.BigEndian.PutUint16(buf[8+32:8+34], width)
	binary.BigEndian.PutUint16(buf[8+34:8+36], height)
	return buf
}

// mp4SampleSize builds a minimal stsz body: version/flags,
// sample_size (0, meaning "see the per-sample table") and
// sample_count, the only field this package reads. No per-sample
// entries are appended, since nothing here ever reads them.
func mp4SampleSize(sampleCount uint32) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[8:12], sampleCount)
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

// webmParams names everything webmFixture varies between test cases.
type webmParams struct {
	videoCodec    string // defaults to "V_VP9" when empty
	audioCodec    string // "" for no audio track at all
	width         uint64
	height        uint64
	timecodeScale uint64 // defaults to 1_000_000 (1ms) when zero
	durationTicks float64
}

// webmFixture returns a minimal, well-formed WebM file: an EBML
// header, then one Segment holding an Info element (TimecodeScale and
// Duration) and a Tracks element with one video TrackEntry (and, with
// an audioCodec set, one audio TrackEntry too).
func webmFixture(p webmParams) []byte {
	if p.videoCodec == "" {
		p.videoCodec = "V_VP9"
	}
	if p.timecodeScale == 0 {
		p.timecodeScale = 1_000_000
	}

	videoElem := ebmlVideoTrackEntry(p.videoCodec, p.width, p.height)
	tracksBody := videoElem
	if p.audioCodec != "" {
		tracksBody = append(tracksBody, ebmlAudioTrackEntry(p.audioCodec)...)
	}

	info := ebmlElement(testIDTimecodeScale, ebmlUint(p.timecodeScale))
	info = append(info, ebmlElement(testIDDuration, ebmlFloat64(p.durationTicks))...)

	segBody := ebmlElement(testIDInfo, info)
	segBody = append(segBody, ebmlElement(testIDTracks, tracksBody)...)

	out := ebmlElement(testEBMLHeaderID, []byte{}) // EBML header, empty: nothing here reads it
	return append(out, ebmlElement(testIDSegment, segBody)...)
}

// ebmlVideoTrackEntry builds a TrackEntry element for a video track:
// TrackType 1, the given CodecID, and a Video sub-element carrying
// PixelWidth and PixelHeight.
func ebmlVideoTrackEntry(codec string, width, height uint64) []byte {
	video := ebmlElement(testIDPixelWidth, ebmlUint(width))
	video = append(video, ebmlElement(testIDPixelHeight, ebmlUint(height))...)

	entry := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeVideo))
	entry = append(entry, ebmlElement(testIDCodecID, []byte(codec))...)
	entry = append(entry, ebmlElement(testIDVideo, video)...)
	return ebmlElement(testIDTrackEntry, entry)
}

// ebmlAudioTrackEntry builds a TrackEntry element for an audio track:
// TrackType 2 and the given CodecID, with no Video sub-element.
func ebmlAudioTrackEntry(codec string) []byte {
	entry := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeAudio))
	entry = append(entry, ebmlElement(testIDCodecID, []byte(codec))...)
	return ebmlElement(testIDTrackEntry, entry)
}

// ebmlElement wraps body in an EBML element: id encoded as a plain
// big-endian integer of its own natural byte width (matching how
// readID interprets a canonical ID), followed by body's length
// encoded as an 8-byte vint, wide enough for every size this test
// package ever builds.
func ebmlElement(id uint32, body []byte) []byte {
	idBytes := ebmlIDBytes(id)
	size := ebmlSizeVint(uint64(len(body)), 8)
	out := append(append([]byte{}, idBytes...), size...)
	return append(out, body...)
}

// ebmlIDBytes encodes id as the fewest leading bytes that hold its
// value, matching the canonical byte width every ID constant in
// matroska.go is already written with (4 bytes for a 0x1X......
// value, 1 for a bare 0xAE, and so on).
func ebmlIDBytes(id uint32) []byte {
	switch {
	case id > 0x00FFFFFF:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0x0000FFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0x000000FF:
		return []byte{byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id)}
	}
}

// ebmlSizeVint encodes value as an EBML size vint exactly length
// bytes wide, setting the matching marker bit; length must be large
// enough to hold value (8 is always enough for this test package's
// own fixtures).
func ebmlSizeVint(value uint64, length int) []byte {
	buf := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		buf[i] = byte(value)
		value >>= 8
	}
	buf[0] |= 0x80 >> (length - 1)
	return buf
}

// ebmlUint encodes v as an 8-byte big-endian EBML unsigned integer:
// wider than strictly needed, which this format allows, and simpler
// for a test fixture than picking the minimal width each time.
func ebmlUint(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

// ebmlFloat64 encodes v as an 8-byte big-endian IEEE 754 double, the
// EBML float width this package's own floatElement also accepts.
func ebmlFloat64(v float64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, math.Float64bits(v))
	return buf
}
