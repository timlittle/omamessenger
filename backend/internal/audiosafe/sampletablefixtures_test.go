package audiosafe_test

// sampletablefixtures_test.go builds the fixtures sampletables_test.go
// exercises: an mp4a audio track whose own stbl carries exactly the
// sample tables a test names. Mirrors videosafe's own
// sampletablefixtures_test.go.

import "encoding/binary"

// audioTableParams names the stbl children a sample-table consistency
// test builds on the audio track mp4AudioTrackWithTables returns:
// stsz/stz2, stco and stsc are the layout triangle
// sampleTablesConsistent always checks once sampleCount is non-zero;
// stts, ctts, stss, sbgp and sgpd are each omitted (nil) unless a
// test sets them, and edts carries a trak-level edit list the same
// way.
type audioTableParams struct {
	stsz, stz2, stco, co64, stsc []byte
	stts, ctts, stss, sbgp, sgpd []byte
	edts                         []byte
}

// mp4AudioTrackWithTables builds a minimal, well-formed M4A with one
// mp4a audio track whose own stbl carries exactly p's own tables.
func mp4AudioTrackWithTables(p audioTableParams) []byte {
	stblBody := mp4Box("stsd", mp4SampleDescription("mp4a"))
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

	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	minf := mp4Box("minf", mp4Box("stbl", stblBody))
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trakBody := append([]byte{}, mdia...)
	if p.edts != nil {
		trakBody = append(trakBody, p.edts...)
	}
	trak := mp4Box("trak", trakBody)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return append(ftyp, moov...)
}

// stszBody builds a variable-size stsz box body (sample_size 0) with
// the given per-sample sizes.
func stszBody(sizes ...uint32) []byte {
	buf := make([]byte, 12+4*len(sizes))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(sizes)))
	for i, s := range sizes {
		binary.BigEndian.PutUint32(buf[12+i*4:16+i*4], s)
	}
	return buf
}

// fixedStszBody builds a fixed-size stsz box body: sample_size is set
// to size (every sample is exactly that many bytes).
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

// stscBody builds an stsc box body with one entry: firstChunk,
// samplesPerChunk and sample_description_index fixed at 1 - every
// case this package's own tests need.
func stscBody(firstChunk, samplesPerChunk uint32) []byte {
	return stscBodyWithIndex(firstChunk, samplesPerChunk, 1)
}

// stscBodyWithIndex builds an stsc box body like stscBody, but with
// the given sample_description_index instead of always 1 - for a
// test that checks a track is refused when its one chunk's samples
// claim a sample description other than the one entry this package
// ever validates.
func stscBodyWithIndex(firstChunk, samplesPerChunk, index uint32) []byte {
	buf := make([]byte, 8+12)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], firstChunk)
	binary.BigEndian.PutUint32(buf[12:16], samplesPerChunk)
	binary.BigEndian.PutUint32(buf[16:20], index)
	return buf
}

// stscBodyTwoEntries builds an stsc box body with two entries, each
// (firstChunk, samplesPerChunk), sample_description_index fixed at 1.
func stscBodyTwoEntries(firstChunk1, samplesPerChunk1, firstChunk2, samplesPerChunk2 uint32) []byte {
	buf := make([]byte, 8+24)
	binary.BigEndian.PutUint32(buf[4:8], 2) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], firstChunk1)
	binary.BigEndian.PutUint32(buf[12:16], samplesPerChunk1)
	binary.BigEndian.PutUint32(buf[16:20], 1)
	binary.BigEndian.PutUint32(buf[20:24], firstChunk2)
	binary.BigEndian.PutUint32(buf[24:28], samplesPerChunk2)
	binary.BigEndian.PutUint32(buf[28:32], 1)
	return buf
}

// sttsBody builds an stts box body with one (sampleCount, sampleDelta)
// entry.
func sttsBody(sampleCount, sampleDelta uint32) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], sampleCount)
	binary.BigEndian.PutUint32(buf[12:16], sampleDelta)
	return buf
}

// cttsBody builds a ctts box body from (sampleCount, sampleOffset)
// pairs, one 8-byte entry per pair.
func cttsBody(pairs ...uint32) []byte {
	entryCount := len(pairs) / 2
	buf := make([]byte, 8+8*entryCount)
	binary.BigEndian.PutUint32(buf[4:8], uint32(entryCount))
	for i := 0; i < entryCount; i++ {
		binary.BigEndian.PutUint32(buf[8+i*8:12+i*8], pairs[i*2])
		binary.BigEndian.PutUint32(buf[12+i*8:16+i*8], pairs[i*2+1])
	}
	return buf
}

// stssBody builds an stss box body from the given sample numbers
// (1-based), one 4-byte entry per number.
func stssBody(sampleNumbers ...uint32) []byte {
	buf := make([]byte, 8+4*len(sampleNumbers))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(sampleNumbers)))
	for i, n := range sampleNumbers {
		binary.BigEndian.PutUint32(buf[8+i*4:12+i*4], n)
	}
	return buf
}

// sbgpBody builds a version-0 sbgp box body with one (sampleCount,
// groupDescriptionIndex) entry: version/flags(4) + grouping_type(4,
// never itself read) + entry_count(4), then the one entry.
func sbgpBody(sampleCount, groupDescriptionIndex uint32) []byte {
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[8:12], 1) // entry_count
	binary.BigEndian.PutUint32(buf[12:16], sampleCount)
	binary.BigEndian.PutUint32(buf[16:20], groupDescriptionIndex)
	return buf
}

// sgpdBody builds a version-1 sgpd box body for grouping_type "roll"
// with defaultLength set to the common 2-byte roll_distance shape,
// and descriptionCount descriptions each exactly that long.
func sgpdBody(descriptionCount uint32) []byte {
	const defaultLength = 2
	out := make([]byte, 16+defaultLength*descriptionCount)
	out[0] = 1 // version
	copy(out[4:8], "roll")
	binary.BigEndian.PutUint32(out[8:12], defaultLength)
	binary.BigEndian.PutUint32(out[12:16], descriptionCount)
	return out
}

// sbgpBodyV1 builds a version-1 sbgp box body (grouping_type_parameter
// before entry_count) with one (sampleCount, groupDescriptionIndex)
// entry.
func sbgpBodyV1(sampleCount, groupDescriptionIndex uint32) []byte {
	buf := make([]byte, 24)
	buf[0] = 1                                // version
	binary.BigEndian.PutUint32(buf[12:16], 1) // entry_count
	binary.BigEndian.PutUint32(buf[16:20], sampleCount)
	binary.BigEndian.PutUint32(buf[20:24], groupDescriptionIndex)
	return buf
}

// sgpdBodyVariableLength builds a version-1 sgpd box body with
// default_length 0 (ISO/IEC 14496-12's "variable length" shape, each
// description carrying its own 4-byte length prefix) and declaredCount
// as its own entry_count, regardless of how many description bytes
// entries actually holds.
func sgpdBodyVariableLength(entries []byte, declaredCount uint32) []byte {
	out := make([]byte, 16+len(entries))
	out[0] = 1 // version
	copy(out[4:8], "roll")
	// default_length (out[8:12]) left 0: variable length.
	binary.BigEndian.PutUint32(out[12:16], declaredCount)
	copy(out[16:], entries)
	return out
}

// elstBodyV1 builds a version-1 elst box body (8-byte segment_duration
// and media_time) with exactly one entry.
func elstBodyV1(segmentDuration uint64) []byte {
	buf := make([]byte, 24)
	buf[0] = 1                              // version
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint64(buf[8:16], segmentDuration)
	return buf
}

// elstBody builds an elst box body (version 0) with the given number
// of entries, each declaring segmentDuration (in the movie's own
// timescale units) and media_time 0.
func elstBody(entryCount, segmentDuration uint32) []byte {
	buf := make([]byte, 8+12*entryCount)
	binary.BigEndian.PutUint32(buf[4:8], entryCount)
	for i := uint32(0); i < entryCount; i++ {
		binary.BigEndian.PutUint32(buf[8+i*12:12+i*12], segmentDuration)
		// media_time and media_rate are left zero: this package never
		// reads either.
	}
	return buf
}

// edtsWith wraps body in an edts box carrying one elst child.
func edtsWith(body []byte) []byte {
	return mp4Box("edts", mp4Box("elst", body))
}
