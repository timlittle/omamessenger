package videosafe_test

// sampletablefixtures_test.go builds the fixtures sampletables_test.go
// exercises: an ordinary, valid avc1 video track (zero samples)
// alongside an mp4a audio track whose own stbl carries exactly the
// sample tables a test names, isolating sampleTablesConsistent's own
// cross-checks (isobmff.go's own trackTablesAndEditsOK) from avc1's
// own in-band NAL scan, which never runs for an audio track at all.

import "encoding/binary"

// sampleTableParams names the stbl children a sample-table
// consistency test builds on the audio track mp4AudioTrackWithTables
// returns: stsz, stco and stsc are the layout triangle
// sampleTablesConsistent always checks once sampleCount is non-zero;
// stts, ctts, stss, sbgp and sgpd are each omitted (nil) unless a
// test sets them, and edts carries a trak-level edit list the same
// way.
type sampleTableParams struct {
	stsz, stco, stsc             []byte
	stts, ctts, stss, sbgp, sgpd []byte
	edts                         []byte
}

// mp4AudioTrackWithTables builds a minimal, well-formed MP4 with one
// ordinary, valid avc1 video track (zero samples, so it never reaches
// its own in-band NAL scan) and one mp4a audio track whose own stbl
// carries exactly p's own tables.
func mp4AudioTrackWithTables(p sampleTableParams) []byte {
	videoTrak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})

	stblBody := mp4Box("stsd", mp4SampleDescription("mp4a", 0, 0))
	type namedBox struct {
		boxType string
		body    []byte
	}
	for _, b := range []namedBox{
		{"stsz", p.stsz},
		{"stco", p.stco},
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

	audioMdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	audioHdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	audioMinf := mp4Box("minf", mp4Box("stbl", stblBody))
	audioMdia := mp4Box("mdia", concatBytes(audioMdhd, audioHdlr, audioMinf))
	audioTrakBody := append([]byte{}, audioMdia...)
	if p.edts != nil {
		audioTrakBody = append(audioTrakBody, p.edts...)
	}
	audioTrak := mp4Box("trak", audioTrakBody)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", concatBytes(mvhd, videoTrak, audioTrak))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return concatBytes(ftyp, moov)
}

// sttsBody builds an stts box body from (sampleCount, sampleDelta)
// pairs, one 8-byte entry per pair.
func sttsBody(entries ...[2]uint32) []byte {
	buf := make([]byte, 8+8*len(entries))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(entries)))
	for i, e := range entries {
		binary.BigEndian.PutUint32(buf[8+i*8:12+i*8], e[0])
		binary.BigEndian.PutUint32(buf[12+i*8:16+i*8], e[1])
	}
	return buf
}

// cttsBody builds a ctts box body from (sampleCount, sampleOffset)
// pairs, the same 8-byte-row shape sttsBody builds.
func cttsBody(entries ...[2]uint32) []byte {
	return sttsBody(entries...)
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
// entries actually holds - letting a test build a count that lies
// about what is really there.
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

// sbgpBody builds a version-0 sbgp box body from (sampleCount,
// groupDescriptionIndex) pairs, one 8-byte entry per pair: version/
// flags(4) + grouping_type(4, never itself read) + entry_count(4),
// then the entries.
func sbgpBody(entries ...[2]uint32) []byte {
	buf := make([]byte, 12+8*len(entries))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(entries)))
	for i, e := range entries {
		binary.BigEndian.PutUint32(buf[12+i*8:16+i*8], e[0])
		binary.BigEndian.PutUint32(buf[16+i*8:20+i*8], e[1])
	}
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
