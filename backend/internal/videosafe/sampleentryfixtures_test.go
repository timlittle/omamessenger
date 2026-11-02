package videosafe_test

// sampleentryfixtures_test.go builds the audio sample entry fixtures
// this package's own tests exercise, and the avc1 sample entry
// fixtures sampleentry_test.go's own avc1 test exercises: a real,
// full-length AudioSampleEntry carrying a real esds (mp4a) or dOps
// (Opus) configuration box, an avc1 entry whose own children past
// avcC are given directly, and the pieces a test needs to shape
// either one a specific, otherwise unreachable way.

import "encoding/binary"

// binaryPutDims writes width and height into entryBody at the fixed
// offset every VisualSampleEntry fixture in this package already uses
// (24 and 26 bytes in): the one field buildCustomAVCFixtureWithEntryBody's
// own caller needs set after building entryBody's own full byte
// sequence, including any extra boxes placed ahead of avcC.
func binaryPutDims(entryBody []byte, width, height int) {
	binary.BigEndian.PutUint16(entryBody[24:26], uint16(width))
	binary.BigEndian.PutUint16(entryBody[26:28], uint16(height))
}

// buildCustomAVCFixtureWithEntryBody builds a minimal, well-formed
// MP4 whose one avc1 sample entry's own body, past its fixed
// VisualSampleEntry fields, is exactly entryBody: for a test that
// needs the entry's own children shaped a specific, otherwise
// unreachable way (an extra box ahead of or behind avcC, or more than
// one avcC).
func buildCustomAVCFixtureWithEntryBody(width, height uint16, entryBody []byte) []byte {
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

// mp4FixtureWithAudioEntry builds a minimal, well-formed MP4 with one
// ordinary, valid avc1 video track and one audio track of codec whose
// own sample entry carries exactly children past its own fixed
// AudioSampleEntry fields (see mp4AudioSampleEntryWithChildren).
func mp4FixtureWithAudioEntry(codec string, children []byte) []byte {
	videoTrak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})

	audioStsd := mp4Box("stsd", mp4AudioSampleEntryStsd(codec, children))
	audioStsz := mp4Box("stsz", mp4SampleSize(0))
	audioStbl := mp4Box("stbl", append(audioStsd, audioStsz...))
	audioMinf := mp4Box("minf", audioStbl)
	audioMdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	audioHdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	audioMdia := mp4Box("mdia", concatBytes(audioMdhd, audioHdlr, audioMinf))
	audioTrak := mp4Box("trak", audioMdia)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", concatBytes(mvhd, videoTrak, audioTrak))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return concatBytes(ftyp, moov)
}

// mp4AudioSampleEntryStsd wraps mp4AudioSampleEntryWithChildren's own
// entry in an stsd body declaring exactly one sample entry.
func mp4AudioSampleEntryStsd(codec string, children []byte) []byte {
	entry := mp4AudioSampleEntryWithChildren(codec, children)

	buf := make([]byte, 8, 8+len(entry))
	buf[7] = 1 // entry_count
	return append(buf, entry...)
}

// mp4AudioSampleDescription builds an stsd body holding one audio
// sample entry (mp4a or Opus) at its full, real AudioSampleEntry
// fixed-field length (28 bytes), followed by a real configuration box
// - esds declaring AAC-LC at 44.1kHz stereo for mp4a (see
// buildAACESDS), or dOps for Opus - since Validate now requires an
// audio sample entry to carry exactly the one configuration box a
// real encoder writes (see sampleentry.go and docs/decisions.md).
func mp4AudioSampleDescription(codec string) []byte {
	var config []byte
	switch codec {
	case "mp4a":
		config = mp4Box("esds", buildAACESDS(2, 4, 2)) // AAC LC, 44.1kHz, stereo
	case "Opus":
		config = mp4Box("dOps", buildOpusSpecificBox())
	}

	entryBody := append(make([]byte, 28), config...)
	entry := mp4Box(codec, entryBody)

	buf := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	return append(buf, entry...)
}

// mp4AudioSampleEntryWithChildren builds one audio sample entry (28
// fixed bytes, then children given directly) for codec: for a test
// that needs the entry's own children shaped a specific, otherwise
// unreachable way (missing, duplicated, or an esds that does not
// describe AAC).
func mp4AudioSampleEntryWithChildren(codec string, children []byte) []byte {
	entryBody := append(make([]byte, 28), children...)
	return mp4Box(codec, entryBody)
}

// buildAACESDS builds a minimal esds box body (ISO/IEC 14496-1's own
// ES_Descriptor, nested DecoderConfigDescriptor and
// DecoderSpecificInfo): objectTypeIndication 0x40 (MPEG-4 Audio) and
// an AudioSpecificConfig built from objectType, freqIndex and
// channelConfig exactly the way audioSpecificConfigAllowed reads them
// back - the inverse of that function's own bit layout.
func buildAACESDS(objectType, freqIndex, channelConfig byte) []byte {
	asc := []byte{
		(objectType << 3) | (freqIndex >> 1),
		((freqIndex & 1) << 7) | (channelConfig << 3),
	}
	dsi := mp4Descriptor(0x05, asc)
	dcd := append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, dsi...) // objectTypeIndication, then 12 fixed bytes
	dcdDesc := mp4Descriptor(0x04, dcd)
	es := append([]byte{0, 0, 0}, dcdDesc...) // ES_ID(2)=0, flags(1)=0
	esDesc := mp4Descriptor(0x03, es)
	return append([]byte{0, 0, 0, 0}, esDesc...) // version/flags
}

// mp4Descriptor wraps body in an MPEG-4 descriptor (ISO/IEC 14496-1):
// tag, then body's own length as a single base-128 byte, always under
// 128 for every descriptor this test package ever builds.
func mp4Descriptor(tag byte, body []byte) []byte {
	return append([]byte{tag, byte(len(body))}, body...)
}

// mp4DescriptorLong wraps body in an MPEG-4 descriptor using a fixed
// 2-byte base-128 length (continuation bit set on the first byte),
// for a body at or above 128 bytes - the multi-byte shape
// readMP4DescriptorHeader must also walk, which mp4Descriptor's own
// single byte never reaches.
func mp4DescriptorLong(tag byte, body []byte) []byte {
	n := len(body)
	return append([]byte{tag, 0x80 | byte(n>>7), byte(n & 0x7F)}, body...)
}

// buildAACESDSPadded builds an esds box body exactly like
// buildAACESDS, with padBytes of trailing zero padding appended to
// the AudioSpecificConfig: ignored by audioSpecificConfigAllowed,
// which never reads past the first two bytes, but large enough to
// push the DecoderSpecificInfo's, the DecoderConfigDescriptor's and
// the ES_Descriptor's own declared lengths all past 127, so building
// one with a large enough padBytes reaches mp4DescriptorLong's own
// shape at every nesting level at once.
func buildAACESDSPadded(objectType, freqIndex, channelConfig byte, padBytes int) []byte {
	asc := append([]byte{
		(objectType << 3) | (freqIndex >> 1),
		((freqIndex & 1) << 7) | (channelConfig << 3),
	}, make([]byte, padBytes)...)

	dsi := mp4DescriptorLong(0x05, asc)
	dcd := append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, dsi...)
	dcdDesc := mp4DescriptorLong(0x04, dcd)
	es := append([]byte{0, 0, 0}, dcdDesc...)
	esDesc := mp4DescriptorLong(0x03, es)
	return append([]byte{0, 0, 0, 0}, esDesc...)
}

// buildAACESDSExplicitRate builds a minimal esds box body whose
// AudioSpecificConfig uses samplingFrequencyIndex 15 (the escape
// value) and an explicit 24-bit sample rate, rather than one of the
// standard table's 13 entries: the shape explicitAACSampleRateAllowed
// judges, which buildAACESDS's own fixed 2-byte AudioSpecificConfig
// never reaches.
func buildAACESDSExplicitRate(objectType byte, rate uint32, channelConfig byte) []byte {
	asc := []byte{
		(objectType << 3) | 0x07, // audioObjectType, then the top 3 bits of freqIndex 15 (0b111)
		0x80 | byte(rate>>17),    // low bit of freqIndex 15 (0b1), then the rate's own top 7 bits
		byte(rate >> 9),
		byte(rate >> 1),
		(byte(rate&1) << 7) | (channelConfig << 3),
	}
	dsi := mp4Descriptor(0x05, asc)
	dcd := append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, dsi...)
	dcdDesc := mp4Descriptor(0x04, dcd)
	es := append([]byte{0, 0, 0}, dcdDesc...)
	esDesc := mp4Descriptor(0x03, es)
	return append([]byte{0, 0, 0, 0}, esDesc...)
}

// buildOpusSpecificBox builds a minimal dOps box body: just the
// version byte (0, the only value this package accepts) and enough
// zero bytes to cover OpusSpecificBox's own remaining fixed fields,
// none of which this package reads.
func buildOpusSpecificBox() []byte {
	return make([]byte, 11)
}

// mp4FixtureWithTinyAudioEntry builds a minimal, well-formed MP4
// whose one audio track's sample entry body is only bodyLen bytes -
// short of audioSampleEntryFixedFields(28) when bodyLen is - for a
// test that checks audioSampleEntry refuses an entry too short to
// even hold its own fixed fields.
func mp4FixtureWithTinyAudioEntry(codec string, bodyLen int) []byte {
	entry := mp4Box(codec, make([]byte, bodyLen))

	stsdBody := make([]byte, 8, 8+len(entry))
	binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
	stsdBody = append(stsdBody, entry...)

	stsz := mp4Box("stsz", mp4SampleSize(0))
	stbl := mp4Box("stbl", append(mp4Box("stsd", stsdBody), stsz...))
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trak := mp4Box("trak", mdia)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	out := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return append(out, moov...)
}

// buildESDSWithObjectTypeIndication builds an esds box body whose
// DecoderConfigDescriptor declares objectTypeIndication directly,
// with an otherwise ordinary AAC LC AudioSpecificConfig behind it -
// for a test that checks this package refuses an object type other
// than 0x40 regardless of what the AudioSpecificConfig behind it
// says.
func buildESDSWithObjectTypeIndication(objectTypeIndication byte) []byte {
	esds := buildAACESDS(2, 4, 2)
	// buildAACESDS's own objectTypeIndication byte sits right after
	// version/flags(4) + ES_Descriptor header(2) + ES_ID/flags(3) +
	// DecoderConfigDescriptor header(2): byte 11.
	esds[11] = objectTypeIndication
	return esds
}

// sampleTableAndEntryFuzzSeeds returns the fuzz seeds
// FuzzValidateMP4 adds for this security fix's own new checks: an
// audio track's stts total disagreeing with its own stsz sample
// count, a trak-level edit list with more than one entry, an mp4a
// esds declaring a non-AAC object type, and an avc1 sample entry
// carrying a second, replacement decoder configuration alongside its
// trusted avcC.
func sampleTableAndEntryFuzzSeeds() [][]byte {
	sttsMismatch := baselineAudioTables()
	sttsMismatch.stts = sttsBody([2]uint32{85_000_000, 1024})

	multiEntryEdit := baselineAudioTables()
	multiEntryEdit.edts = edtsWith(elstBody(2, 5000))

	entryBody := make([]byte, 78)
	binaryPutDims(entryBody, 1280, 720)
	entryBody = append(entryBody, mp4Box("avcC", buildAVCDecoderConfig(1280, 720))...)
	entryBody = append(entryBody, mp4Box("esds", []byte("not a real esds"))...)

	return [][]byte{
		mp4AudioTrackWithTables(sttsMismatch),
		mp4AudioTrackWithTables(multiEntryEdit),
		mp4FixtureWithAudioEntry("mp4a", mp4Box("esds", buildESDSWithObjectTypeIndication(0x6B))),
		buildCustomAVCFixtureWithEntryBody(1280, 720, entryBody),
	}
}
