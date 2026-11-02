package videosafe_test

// avcsamplefixtures_test.go builds the avc1 sample-layout fixtures
// avcsamples_test.go exercises: real, length-prefixed NAL units
// placed in mdat and located through stsz/stco/co64/stsc, split out
// of helpers_test.go once the general MP4/WebM builders there and
// these avc1-sample-specific ones together grew past this package's
// own file-length limit.

import (
	"bytes"
	"encoding/binary"
)

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
	return mp4AVCVideoFixtureWithAVCC(p.width, p.height, buildAVCDecoderConfig(p.width, p.height), p.samples)
}

// mp4AVCVideoFixtureWithAVCC builds the same fixture shape as
// mp4AVCVideoWithSamples, but with avcC's own body given directly
// rather than always built from width/height through
// buildAVCDecoderConfig: for a test that needs a track declaring a
// NAL length-prefix width other than 4 bytes (see
// buildAVCDecoderConfigWithLengthSize), the exact shape the "a sample
// shaped like avcC itself" exploit in avcsamples_test.go needs a 1-
// or 2-byte prefix to reach.
func mp4AVCVideoFixtureWithAVCC(width, height int, avccBody []byte, samples [][]byte) []byte {
	entryBody := make([]byte, 78)
	binary.BigEndian.PutUint16(entryBody[24:26], uint16(width))
	binary.BigEndian.PutUint16(entryBody[26:28], uint16(height))
	entryBody = append(entryBody, mp4Box("avcC", avccBody)...)
	entry := mp4Box("avc1", entryBody)

	stsdBody := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
	stsdBody = append(stsdBody, entry...)

	stbl := mp4Box("stbl", concatBytes(
		mp4Box("stsd", stsdBody),
		mp4Box("stsz", mp4SampleSizeTable(samples)),
		mp4Box("stsc", mp4SampleToChunk(uint32(len(samples)))),
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
	mdat := mp4Box("mdat", concatBytes(samples...))

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
	stts          []byte
	ctts          []byte
	stss          []byte
	sbgp          []byte
	sgpd          []byte
	edts          []byte
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
	for _, b := range []namedBox{
		{"stsz", p.stsz},
		{"stz2", p.stz2},
		{"stco", p.stco},
		{"co64", p.co64},
		{"stsc", p.stsc},
		{"stts", p.stts},
		{"ctts", p.ctts},
		{"stss", p.stss},
		{"sbgp", p.sbgp},
		{"sgpd", p.sgpd},
	} {
		if b.body != nil {
			stblBody = append(stblBody, mp4Box(b.boxType, b.body)...)
		}
	}

	stbl := mp4Box("stbl", stblBody)
	minf := mp4Box("minf", stbl)
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
	mdia := mp4Box("mdia", concatBytes(mdhd, hdlr, minf))
	trakBody := append([]byte{}, mdia...)
	if p.edts != nil {
		trakBody = append(trakBody, p.edts...)
	}
	trak := mp4Box("trak", trakBody)
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
	return stscBodyWithIndex(1, entries...)
}

// stscBodyWithIndex builds an stsc box body like stscBody, but with
// the given sample_description_index on every entry instead of
// always 1 - for a test that checks a track is refused when a chunk's
// samples claim a sample description other than the one entry this
// package ever validates (see sampleEntryCodec's own entry_count
// check and stscEntriesValid).
func stscBodyWithIndex(index uint32, entries ...[2]uint32) []byte {
	buf := make([]byte, 8+12*len(entries))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(entries)))
	for i, e := range entries {
		binary.BigEndian.PutUint32(buf[8+i*12:12+i*12], e[0])
		binary.BigEndian.PutUint32(buf[12+i*12:16+i*12], e[1])
		binary.BigEndian.PutUint32(buf[16+i*12:20+i*12], index)
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
