package videosafe_test

// helpers_test.go builds minimal, well-formed MP4/MOV and
// WebM/Matroska byte streams for the table and fuzz tests in
// isobmff_test.go and matroska_test.go to start from and mutate: each
// builder takes the handful of fields this package actually reads
// (codec, dimensions, duration, sample count) so a test can construct
// exactly the file it wants to check, rather than depending on a
// real-world sample file checked into the repository.

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/bits"
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
// layout. For "avc1", the entry also carries a real avcC box with a
// matching SPS, since Validate now requires the two to agree: see
// mp4AVCSampleDescription.
func mp4SampleDescription(codec string, width, height uint16) []byte {
	if codec == "avc1" {
		return mp4AVCSampleDescription(width, height)
	}

	buf := make([]byte, 8+44)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], 44)
	copy(buf[12:16], codec)
	binary.BigEndian.PutUint16(buf[8+32:8+34], width)
	binary.BigEndian.PutUint16(buf[8+34:8+36], height)
	return buf
}

// mp4AVCSampleDescription builds an stsd body holding one avc1
// sample entry at its full, real VisualSampleEntry fixed-field length
// (78 bytes - this package's own avcSPSDims refuses anything
// shorter), followed by an avcC box whose single SPS decodes to
// exactly width and height, through buildAVCSPS.
func mp4AVCSampleDescription(width, height uint16) []byte {
	entryBody := make([]byte, 78)
	binary.BigEndian.PutUint16(entryBody[24:26], width)
	binary.BigEndian.PutUint16(entryBody[26:28], height)
	entryBody = append(entryBody, mp4Box("avcC", buildAVCDecoderConfig(int(width), int(height)))...)
	entry := mp4Box("avc1", entryBody)

	buf := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	return append(buf, entry...)
}

// buildCustomAVCFixture builds a minimal, well-formed MP4 whose avc1
// sample entry declares containerWidth/containerHeight directly and
// whose avcC box body is exactly avccBody, letting a test construct
// a container/SPS mismatch directly rather than through
// mp4AVCSampleDescription's own automatic match.
func buildCustomAVCFixture(containerWidth, containerHeight uint16, avccBody []byte) []byte {
	entryBody := make([]byte, 78)
	binary.BigEndian.PutUint16(entryBody[24:26], containerWidth)
	binary.BigEndian.PutUint16(entryBody[26:28], containerHeight)
	entryBody = append(entryBody, mp4Box("avcC", avccBody)...)
	entry := mp4Box("avc1", entryBody)

	stsdBody := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
	stsdBody = append(stsdBody, entry...)

	stsz := mp4Box("stsz", mp4SampleSize(0))
	stbl := mp4Box("stbl", append(mp4Box("stsd", stsdBody), stsz...))
	minf := mp4Box("minf", stbl)
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trak := mp4Box("trak", mdia)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	out := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return append(out, moov...)
}

// duplicateSPSAVCConfig builds an AVCDecoderConfigurationRecord that
// declares two sequence parameter sets, both the same bytes a single
// SPS for width/height would be: enough to check that Validate
// refuses any count other than exactly one, regardless of whether
// the declared sets happen to agree with each other.
func duplicateSPSAVCConfig(width, height int) []byte {
	sps := buildAVCSPS(width, height)
	spsLen := make([]byte, 2)
	binary.BigEndian.PutUint16(spsLen, uint16(len(sps)))

	buf := []byte{1, 66, 0, 30, 0xFF, 0x02} // declares two sequence parameter sets
	buf = append(buf, spsLen...)
	buf = append(buf, sps...)
	buf = append(buf, spsLen...)
	return append(buf, sps...)
}

// buildAVCDecoderConfig builds an AVCDecoderConfigurationRecord
// (avcC's own body) declaring exactly one SPS, from buildAVCSPS, and
// no picture parameter sets: this package's own avcDecoderConfigSPSDims
// never reads past the one SPS it requires.
func buildAVCDecoderConfig(width, height int) []byte {
	sps := buildAVCSPS(width, height)

	buf := []byte{1, 66, 0, 30, 0xFF, 0x01} // version, profile, compat, level and lengthSizeMinusOne, then one declared SPS
	spsLen := make([]byte, 2)
	binary.BigEndian.PutUint16(spsLen, uint16(len(sps)))
	buf = append(buf, spsLen...)
	return append(buf, sps...)
}

// buildAVCSPS builds a single baseline-profile H.264 sequence
// parameter set NAL unit (NAL header byte included) that decodes,
// through this package's own parseSPSDims, to exactly width and
// height: frame_mbs_only_flag set, and frame cropping applied only
// when the macroblock padding on each axis is evenly divisible by the
// 4:2:0 chroma crop unit (2), the one shape a real encoder would ever
// produce. A caller after an exact match (any width and height built
// from mp4Fixture's own dimensions, always chosen to fit this) gets
// one; a caller testing a rejected dimension or edge case does not
// need one to exercise what it is actually testing.
func buildAVCSPS(width, height int) []byte {
	mbWidth := max((width+15)/16, 1)
	mbHeightUnits := max((height+15)/16, 1)

	w := &spsBitWriter{}
	w.u(8, 66) // profile_idc: Baseline, no chroma extension fields
	w.u(8, 0)  // constraint flags + reserved
	w.u(8, 30) // level_idc
	w.ue(0)    // seq_parameter_set_id
	w.ue(0)    // log2_max_frame_num_minus4
	w.ue(0)    // pic_order_cnt_type
	w.ue(0)    // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0)    // max_num_ref_frames
	w.u(1, 0)  // gaps_in_frame_num_value_allowed_flag
	w.ue(uint32(mbWidth - 1))
	w.ue(uint32(mbHeightUnits - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag

	padRight := mbWidth*16 - width
	padBottom := mbHeightUnits*16 - height
	if padRight%2 == 0 && padBottom%2 == 0 && (padRight > 0 || padBottom > 0) {
		w.u(1, 1) // frame_cropping_flag
		w.ue(0)
		w.ue(uint32(padRight / 2))
		w.ue(0)
		w.ue(uint32(padBottom / 2))
	} else {
		w.u(1, 0) // frame_cropping_flag
	}

	return append([]byte{0x67}, addEmulationPrevention(w.bytes())...)
}

// addEmulationPrevention inserts H.264's own emulation-prevention
// byte (0x03) after every two 0x00 bytes immediately followed by a
// byte of 0x03 or less, the exact inverse of this package's own
// removeEmulationPrevention, so a fixture built from arbitrary bits
// round-trips through the real parser exactly as a genuine encoder's
// output would.
func addEmulationPrevention(b []byte) []byte {
	out := make([]byte, 0, len(b)+4)
	zeroRun := 0
	for _, v := range b {
		if zeroRun >= 2 && v <= 0x03 {
			out = append(out, 0x03)
			zeroRun = 0
		}
		out = append(out, v)
		if v == 0 {
			zeroRun++
		} else {
			zeroRun = 0
		}
	}
	return out
}

// spsBitWriter builds an H.264 RBSP one bit at a time, the test
// package's own inverse of the production bitReader, for buildAVCSPS
// to encode exactly the fields parseSPSDims reads.
type spsBitWriter struct {
	bitCount int
	buf      []byte
}

// u appends v's own low n bits, most significant bit first.
func (w *spsBitWriter) u(n int, v uint32) {
	for i := n - 1; i >= 0; i-- {
		w.bit((v >> i) & 1)
	}
}

// ue appends v Exp-Golomb-coded (H.264's own ue(v)).
func (w *spsBitWriter) ue(v uint32) {
	codeNum := v + 1
	leadingZeros := bits.Len32(codeNum) - 1
	for range leadingZeros {
		w.bit(0)
	}
	w.bit(1)
	for i := leadingZeros - 1; i >= 0; i-- {
		w.bit((codeNum >> i) & 1)
	}
}

// bit appends a single bit, growing buf by a byte whenever the
// current one is full.
func (w *spsBitWriter) bit(b uint32) {
	if w.bitCount%8 == 0 {
		w.buf = append(w.buf, 0)
	}
	if b != 0 {
		w.buf[len(w.buf)-1] |= 1 << (7 - w.bitCount%8)
	}
	w.bitCount++
}

// bytes returns every byte written so far, zero-padded to a whole
// byte if needed.
func (w *spsBitWriter) bytes() []byte {
	return w.buf
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

// avcLengthSize is the NAL length-prefix width every fixture in this
// file builds its avcC configuration with (buildAVCDecoderConfig's
// own lengthSizeMinusOne byte, 0xFF, encodes a width of 4).
const avcLengthSize = 4

// avcSample length-prefixes each of nalUnits with avcLengthSize bytes
// and concatenates them, exactly the shape a real avc1 sample's own
// bytes take: one or more NAL units, each preceded by its own length.
func avcSample(nalUnits ...[]byte) []byte {
	var out []byte
	for _, nal := range nalUnits {
		lenBuf := make([]byte, avcLengthSize)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(nal)))
		out = append(out, lenBuf...)
		out = append(out, nal...)
	}
	return out
}

// buildAVCSliceNAL returns a minimal NAL unit of the given type
// (never 7, 8, 13 or 15 - this package's own in-band checks only
// judge those), just long enough to exercise avcNALAllowed's "pass
// through unjudged" default case.
func buildAVCSliceNAL(nalType byte) []byte {
	return []byte{nalType, 0x01, 0x02, 0x03}
}

// buildAVCPPS builds a minimal picture parameter set NAL unit (type
// 8) declaring pic_parameter_set_id 0 and the given
// seq_parameter_set_id, the two fields ppsReferencesSPS reads.
func buildAVCPPS(seqParameterSetID uint32) []byte {
	w := &spsBitWriter{}
	w.ue(0) // pic_parameter_set_id
	w.ue(seqParameterSetID)
	return append([]byte{0x68}, addEmulationPrevention(w.bytes())...)
}

// avcVideoSamplesParams names everything mp4AVCVideoWithSamples
// varies: the avcC SPS's own width and height, and each sample's own
// already-length-prefixed bytes (see avcSample).
type avcVideoSamplesParams struct {
	width, height int
	samples       [][]byte
}

// mp4AVCVideoWithSamples builds a minimal, well-formed MP4 with one
// avc1 video track whose avcC matches width/height, real sample data
// in mdat (one chunk holding every sample, in order, exactly as
// avcsamples.go's own sampleSpans expects to find it through
// stsz/stco/stsc) and no audio track. This is the one fixture shape
// in this package that reaches avcsamples.go's own in-band NAL scan
// at all: every other fixture here uses zero samples, which the scan
// always passes vacuously.
func mp4AVCVideoWithSamples(p avcVideoSamplesParams) []byte {
	entryBody := make([]byte, 78)
	binary.BigEndian.PutUint16(entryBody[24:26], uint16(p.width))
	binary.BigEndian.PutUint16(entryBody[26:28], uint16(p.height))
	entryBody = append(entryBody, mp4Box("avcC", buildAVCDecoderConfig(p.width, p.height))...)
	entry := mp4Box("avc1", entryBody)

	stsdBody := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
	stsdBody = append(stsdBody, entry...)

	stbl := mp4Box("stbl", concatBytes(
		mp4Box("stsd", stsdBody),
		mp4Box("stsz", mp4SampleSizeTable(p.samples)),
		mp4Box("stsc", mp4SampleToChunk(uint32(len(p.samples)))),
		mp4Box("stco", make([]byte, 12)), // entry_count and offset patched below
	))
	minf := mp4Box("minf", stbl)
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
	mdia := mp4Box("mdia", concatBytes(mdhd, hdlr, minf))
	trak := mp4Box("trak", mdia)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", concatBytes(mvhd, trak))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))

	patchStcoOffset(moov, int64(len(ftyp)+len(moov)+8)) // mdat's own body starts right after its 8-byte header
	mdat := mp4Box("mdat", concatBytes(p.samples...))

	return concatBytes(ftyp, moov, mdat)
}

// mp4SampleSizeTable builds an stsz body with sample_size 0 (a
// per-sample table follows) and each sample's own byte length, the
// exact shape avcsamples.go's own sampleSizeTable reads.
func mp4SampleSizeTable(samples [][]byte) []byte {
	buf := make([]byte, 12+4*len(samples))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(samples)))
	for i, s := range samples {
		binary.BigEndian.PutUint32(buf[12+i*4:16+i*4], uint32(len(s)))
	}
	return buf
}

// mp4SampleToChunk builds an stsc body with a single entry: every
// one of count samples lives in chunk 1, sample description index 1.
func mp4SampleToChunk(count uint32) []byte {
	buf := make([]byte, 8+12)
	binary.BigEndian.PutUint32(buf[4:8], 1)       // entry_count
	binary.BigEndian.PutUint32(buf[8:12], 1)      // first_chunk
	binary.BigEndian.PutUint32(buf[12:16], count) // samples_per_chunk
	binary.BigEndian.PutUint32(buf[16:20], 1)     // sample_description_index
	return buf
}

// patchStcoOffset finds the one stco box mp4AVCVideoWithSamples built
// (with a placeholder entry_count of 0) inside moov's own bytes and
// sets it to one chunk at offset, in place: simpler than threading
// the final mdat offset through every box builder above, since
// nothing else in moov's own layout depends on this box's value.
func patchStcoOffset(moov []byte, offset int64) {
	idx := bytes.Index(moov, []byte("stco"))
	body := moov[idx+4:]
	binary.BigEndian.PutUint32(body[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(body[8:12], uint32(offset))
}

// concatBytes concatenates every byte slice in parts into one, for
// the box builders above to assemble a body from several children
// without repeating append chains.
func concatBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// avcCustomLayoutParams lets a test build an avc1 track whose
// stsz/stco/co64/stsc/stz2 boxes are given directly, rather than
// derived from real sample bytes the way mp4AVCVideoWithSamples
// always builds them: for exercising avcsamples.go's own layout
// mismatches and bounds checks (a declared size exceeding the file, a
// missing table, too few chunks for the declared samples) that a
// byte-accurate fixture could never reach on its own. Any field left
// nil omits that box entirely.
type avcCustomLayoutParams struct {
	width, height int
	stsz          []byte
	stz2          []byte
	stco          []byte
	co64          []byte
	stsc          []byte
	mdat          []byte
}

// mp4AVCVideoCustomLayout builds a minimal, well-formed MP4 with one
// avc1 video track whose stbl box carries exactly the layout boxes
// p names, plus a trailing mdat box when p.mdat is set. A chunk
// offset inside p.stco/p.co64 is a placeholder until a test patches
// it in place with patchStcoOffset, patchStcoOffsets or
// patchCo64Offset, once the real file's own layout is known.
func mp4AVCVideoCustomLayout(p avcCustomLayoutParams) []byte {
	entryBody := make([]byte, 78)
	binary.BigEndian.PutUint16(entryBody[24:26], uint16(p.width))
	binary.BigEndian.PutUint16(entryBody[26:28], uint16(p.height))
	entryBody = append(entryBody, mp4Box("avcC", buildAVCDecoderConfig(p.width, p.height))...)
	entry := mp4Box("avc1", entryBody)

	stsdBody := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
	stsdBody = append(stsdBody, entry...)

	stblBody := mp4Box("stsd", stsdBody)
	type namedBox struct {
		boxType string
		body    []byte
	}
	for _, b := range []namedBox{{"stsz", p.stsz}, {"stz2", p.stz2}, {"stco", p.stco}, {"co64", p.co64}, {"stsc", p.stsc}} {
		if b.body != nil {
			stblBody = append(stblBody, mp4Box(b.boxType, b.body)...)
		}
	}

	stbl := mp4Box("stbl", stblBody)
	minf := mp4Box("minf", stbl)
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
	mdia := mp4Box("mdia", concatBytes(mdhd, hdlr, minf))
	trak := mp4Box("trak", mdia)
	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", concatBytes(mvhd, trak))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))

	out := concatBytes(ftyp, moov)
	if p.mdat != nil {
		out = append(out, mp4Box("mdat", p.mdat)...)
	}
	return out
}

// stszBody builds a variable-size stsz box body (sample_size 0) with
// the given per-sample sizes, independent of any real sample bytes -
// for a test that needs stsz to declare a size no sample in the file
// actually has.
func stszBody(sizes ...uint32) []byte {
	buf := make([]byte, 12+4*len(sizes))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(sizes)))
	for i, s := range sizes {
		binary.BigEndian.PutUint32(buf[12+i*4:16+i*4], s)
	}
	return buf
}

// fixedStszBody builds a fixed-size stsz box body: sample_size is
// set to size (every sample is exactly that many bytes), the one
// shape stszBody's own zero sample_size never builds.
func fixedStszBody(size, sampleCount uint32) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[4:8], size)
	binary.BigEndian.PutUint32(buf[8:12], sampleCount)
	return buf
}

// stcoBody builds an stco box body (32-bit chunk offsets).
func stcoBody(offsets ...uint32) []byte {
	buf := make([]byte, 8+4*len(offsets))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(offsets)))
	for i, o := range offsets {
		binary.BigEndian.PutUint32(buf[8+i*4:12+i*4], o)
	}
	return buf
}

// co64Body builds a co64 box body (64-bit chunk offsets).
func co64Body(offsets ...uint64) []byte {
	buf := make([]byte, 8+8*len(offsets))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(offsets)))
	for i, o := range offsets {
		binary.BigEndian.PutUint64(buf[8+i*8:16+i*8], o)
	}
	return buf
}

// stscBody builds an stsc box body from (firstChunk, samplesPerChunk)
// pairs, one entry per pair, sample_description_index fixed at 1.
func stscBody(entries ...[2]uint32) []byte {
	buf := make([]byte, 8+12*len(entries))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(entries)))
	for i, e := range entries {
		binary.BigEndian.PutUint32(buf[8+i*12:12+i*12], e[0])
		binary.BigEndian.PutUint32(buf[12+i*12:16+i*12], e[1])
		binary.BigEndian.PutUint32(buf[16+i*12:20+i*12], 1)
	}
	return buf
}

// patchStcoOffsets finds the one stco box built with as many
// placeholder (zero) offsets as len(offsets) and overwrites them in
// place with the real ones, the same trick patchStcoOffset already
// uses for a single chunk, generalized to however many a test needs.
func patchStcoOffsets(data []byte, offsets []int64) {
	idx := bytes.Index(data, []byte("stco"))
	body := data[idx+4:]
	binary.BigEndian.PutUint32(body[4:8], uint32(len(offsets)))
	for i, off := range offsets {
		binary.BigEndian.PutUint32(body[8+i*4:12+i*4], uint32(off))
	}
}

// patchCo64Offset finds the one co64 box built with a single
// placeholder offset and overwrites it in place with the real one,
// co64's own 64-bit-wide counterpart to patchStcoOffset.
func patchCo64Offset(data []byte, offset int64) {
	idx := bytes.Index(data, []byte("co64"))
	body := data[idx+4:]
	binary.BigEndian.PutUint32(body[4:8], 1) // entry_count
	binary.BigEndian.PutUint64(body[8:16], uint64(offset))
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
