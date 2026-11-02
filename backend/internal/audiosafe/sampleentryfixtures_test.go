package audiosafe_test

// sampleentryfixtures_test.go builds the audio sample entry fixtures
// this package's own tests exercise: a real, full-length
// AudioSampleEntry carrying a real esds (mp4a) or dOps (Opus)
// configuration box, and the pieces a test needs to shape that
// configuration a specific, otherwise unreachable way. Mirrors
// videosafe's own sampleentryfixtures_test.go and helpers_test.go
// additions for the same checks.

import "encoding/binary"

// mp4AudioSampleEntry builds one full, valid audio sample entry (28
// fixed bytes, then a real configuration box) for codec: esds
// declaring AAC LC at 44.1kHz stereo for mp4a, or dOps for Opus.
func mp4AudioSampleEntry(codec string) []byte {
	return mp4AudioSampleEntryWithChildren(codec, mp4AudioSampleEntryDefaultConfig(codec))
}

// mp4AudioSampleEntryDefaultConfig returns the one configuration box
// mp4AudioSampleEntry gives codec by default.
func mp4AudioSampleEntryDefaultConfig(codec string) []byte {
	switch codec {
	case "mp4a":
		return mp4Box("esds", buildAACESDS(2, 4, 2)) // AAC LC, 44.1kHz, stereo
	case "Opus":
		return mp4Box("dOps", buildOpusSpecificBox())
	default:
		return nil
	}
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
// back. Mirrors videosafe's own buildAACESDS.
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
// 128 for every descriptor this test package ever builds. Mirrors
// videosafe's own mp4Descriptor.
func mp4Descriptor(tag byte, body []byte) []byte {
	return append([]byte{tag, byte(len(body))}, body...)
}

// mp4DescriptorLong wraps body in an MPEG-4 descriptor using a fixed
// 2-byte base-128 length (continuation bit set on the first byte),
// for a body at or above 128 bytes. Mirrors videosafe's own
// mp4DescriptorLong.
func mp4DescriptorLong(tag byte, body []byte) []byte {
	n := len(body)
	return append([]byte{tag, 0x80 | byte(n>>7), byte(n & 0x7F)}, body...)
}

// buildAACESDSPadded builds an esds box body exactly like
// buildAACESDS, with padBytes of trailing zero padding appended to
// the AudioSpecificConfig, pushing every enclosing descriptor's own
// declared length past the single-byte encoding. Mirrors videosafe's
// own buildAACESDSPadded.
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
// value) and an explicit 24-bit sample rate. Mirrors videosafe's own
// buildAACESDSExplicitRate.
func buildAACESDSExplicitRate(objectType byte, rate uint32, channelConfig byte) []byte {
	asc := []byte{
		(objectType << 3) | 0x07,
		0x80 | byte(rate>>17),
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

// mp4FixtureWithTinyAudioEntry builds a minimal, well-formed M4A whose
// one audio track's sample entry body is only bodyLen bytes - short
// of audioSampleEntryFixedFields(28) - for a test that checks
// audioSampleEntry refuses an entry too short to even hold its own
// fixed fields.
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
// with an otherwise ordinary AAC LC AudioSpecificConfig behind it.
// Mirrors videosafe's own buildESDSWithObjectTypeIndication.
func buildESDSWithObjectTypeIndication(objectTypeIndication byte) []byte {
	esds := buildAACESDS(2, 4, 2)
	esds[11] = objectTypeIndication
	return esds
}

// mp4AudioFixtureWithEntry builds a minimal, well-formed M4A whose
// one audio track of codec carries exactly children past its own
// fixed AudioSampleEntry fields (see mp4AudioSampleEntryWithChildren),
// alongside an stsz box declaring zero samples, so the one track
// under test is isolated from the sample-table checks
// sampletables_test.go already covers on their own.
func mp4AudioFixtureWithEntry(codec string, children []byte) []byte {
	stsd := mp4Box("stsd", mp4AudioSampleEntryStsd(codec, children))
	stsz := mp4Box("stsz", mp4SampleSize(0))
	stbl := mp4Box("stbl", append(stsd, stsz...))
	mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	minf := mp4Box("minf", stbl)
	mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trak := mp4Box("trak", mdia)

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moov := mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return append(ftyp, moov...)
}

// mp4AudioSampleEntryStsd wraps mp4AudioSampleEntryWithChildren's own
// entry in an stsd body declaring exactly one sample entry.
func mp4AudioSampleEntryStsd(codec string, children []byte) []byte {
	entry := mp4AudioSampleEntryWithChildren(codec, children)

	buf := make([]byte, 8, 8+len(entry))
	buf[7] = 1 // entry_count
	return append(buf, entry...)
}

// buildOpusSpecificBox builds a minimal dOps box body: just the
// version byte (0, the only value this package accepts) and enough
// zero bytes to cover OpusSpecificBox's own remaining fixed fields,
// none of which this package reads. Mirrors videosafe's own
// buildOpusSpecificBox.
func buildOpusSpecificBox() []byte {
	return make([]byte, 11)
}
