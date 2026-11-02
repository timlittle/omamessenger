package videosafe

// sampleentry.go judges what sits *inside* a sample entry, beyond the
// one configuration box this package already trusted: a security
// review found avcConfigDetails located avcC by type alone
// (firstChild), never checking what else the same avc1 entry might
// carry, and that neither package's mp4a/Opus entry was checked past
// its own codec fourcc at all. FFmpeg's own mov demuxer reads every
// child of a sample entry it recognizes, and three of them - an esds,
// an hvcC placed ahead of avcC, or a second avcC - replace the H.264
// configuration this package already validated with one it never
// saw; an mp4a entry's own esds can likewise select any audio decoder
// FFmpeg supports regardless of the track's own stsd codec (see
// docs/decisions.md). The checks here close both gaps: a real
// allowlist for avc1's own children, found by a full walk rather than
// a search for one type among however many others might also be
// there, and a structural read of mp4a's one esds far enough to
// confirm it actually describes AAC.

// avcSampleEntryChildrenOK reports whether entry - an avc1 sample
// entry's own body, already confirmed at least
// avcVisualSampleEntryFixedFields long - carries exactly one avcC box
// among its children and nothing outside allowedAVC1Child, found by
// walking every child in turn rather than asking for avcC alone and
// ignoring the rest: the exact gap that let a replacement
// configuration (hvcC, esds, or a second avcC) sit alongside the one
// avcC this package validated and still reach FFmpeg's own decoder.
func (c *cursor) avcSampleEntryChildrenOK(entry span) bool {
	limit := entry.off + entry.len
	avcCCount := 0
	for off := entry.off + avcVisualSampleEntryFixedFields; off < limit; {
		typ, _, next, ok := c.boxHeader(off, limit)
		if !ok {
			return false
		}
		if !allowedAVC1Child[typ] {
			return false
		}
		if typ == "avcC" {
			avcCCount++
		}
		off = next
	}
	return avcCCount == 1
}

// audioSampleEntry finds stbl's stsd box's one sample entry (already
// confirmed to be the only one stsd declares by sampleEntryCodec) and
// returns its own body, the same search avcConfigDetails runs for a
// video entry.
func (c *cursor) audioSampleEntry(stblOff, stblLen int64) (span, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 8 {
		return span{}, false
	}

	_, entry, _, ok := c.boxHeader(stsd.off+8, stsd.off+stsd.len)
	if !ok || entry.len < audioSampleEntryFixedFields {
		return span{}, false
	}
	return entry, true
}

// audioSampleEntryOK reports whether this track's one audio sample
// entry carries exactly the children a real encoder writes for its
// own codec, and, for mp4a, that its one esds describes AAC. codec is
// always one of allowedISOAudioCodec's own keys by the time this
// runs, so the switch below is exhaustive over what isoTrackAllowed
// already let through.
func (c *cursor) audioSampleEntryOK(stblOff, stblLen int64, codec string) bool {
	entry, ok := c.audioSampleEntry(stblOff, stblLen)
	if !ok {
		return false
	}

	switch codec {
	case "mp4a":
		return c.mp4aSampleEntryOK(entry)
	case "Opus":
		return c.opusSampleEntryOK(entry)
	default:
		return false
	}
}

// mp4aSampleEntryOK reports whether entry carries exactly one esds
// box among children that are all in allowedMP4AChild, and that esds
// describes AAC (see esdsDescribesAAC): a real AAC encoder writes
// nothing outside that set here, and anything else - a second esds,
// or any other box FFmpeg's own mov demuxer might still read as a
// replacement configuration - is refused outright.
func (c *cursor) mp4aSampleEntryOK(entry span) bool {
	esds, ok := c.findOneRequiredChild(entry, "esds", allowedMP4AChild)
	if !ok {
		return false
	}
	return c.esdsDescribesAAC(esds)
}

// opusSampleEntryOK reports whether entry carries exactly one dOps
// box (OpusSpecificBox) among children that are all in
// allowedOpusChild, with its own declared version byte set to the
// only value ISO/IEC 14496-12's own Opus-in-ISOBMFF amendment
// defines.
func (c *cursor) opusSampleEntryOK(entry span) bool {
	dops, ok := c.findOneRequiredChild(entry, "dOps", allowedOpusChild)
	if !ok || dops.len < 1 {
		return false
	}

	var version [1]byte
	if !c.readAt(version[:], dops.off) {
		return false
	}
	return version[0] == 0
}

// findOneRequiredChild walks every child of entry past its own fixed
// AudioSampleEntry fields, refusing outright the moment one is not in
// allowed (the same real-encoder allowlist discipline
// avcSampleEntryChildrenOK already applies to avc1), and returns the
// one child of type required: ok is true only when exactly one such
// child is present, however many of allowed's other box types sit
// alongside it.
func (c *cursor) findOneRequiredChild(entry span, required string, allowed map[string]bool) (span, bool) {
	limit := entry.off + entry.len
	var found span
	count := 0
	for off := entry.off + audioSampleEntryFixedFields; off < limit; {
		typ, body, next, ok := c.boxHeader(off, limit)
		if !ok || !allowed[typ] {
			return span{}, false
		}
		if typ == required {
			found, count = body, count+1
		}
		off = next
	}
	return found, count == 1
}

// esdsDescribesAAC reports whether esds's own ES_Descriptor declares
// a DecoderConfigDescriptor with objectTypeIndication 0x40 (MPEG-4
// Audio, ISO/IEC 14496-3) and a DecoderSpecificInfo whose bytes parse
// as an AudioSpecificConfig this package accepts (see
// audioSpecificConfigAllowed): FFmpeg's own mov demuxer reads esds for
// its decoder type the same way, and a security review found this
// package never checked it at all, leaving esds free to select any
// decoder FFmpeg supports regardless of the track's own stsd codec.
func (c *cursor) esdsDescribesAAC(esds span) bool {
	if esds.len < 4 || esds.len > maxESDSLength {
		return false
	}
	body := make([]byte, esds.len)
	if !c.readAt(body, esds.off) {
		return false
	}

	objectTypeIndication, asc, ok := parseESDescriptor(body[4:])
	if !ok || objectTypeIndication != 0x40 {
		return false
	}
	return audioSpecificConfigAllowed(asc)
}

// parseESDescriptor reads body as an MPEG-4 ES_Descriptor (tag 0x03,
// ISO/IEC 14496-1), assuming none of its own optional flag bits are
// set - streamDependence, URL or OCR - the shape every encoder this
// package has seen write an audio esds uses; any other shape is
// refused outright rather than parsed further. It returns the nested
// DecoderConfigDescriptor's own objectTypeIndication and, when
// present, its own DecoderSpecificInfo bytes (the AudioSpecificConfig).
func parseESDescriptor(body []byte) (objectTypeIndication byte, decoderSpecificInfo []byte, ok bool) {
	tag, length, consumed, ok := readMP4DescriptorHeader(body)
	if !ok || tag != 0x03 {
		return 0, nil, false
	}

	es := body[consumed : consumed+length]
	if len(es) < 3 || es[2] != 0 { // ES_ID(2) + flags(1), every optional-field bit clear
		return 0, nil, false
	}
	return parseDecoderConfigDescriptor(es[3:])
}

// parseDecoderConfigDescriptor reads body as an MPEG-4
// DecoderConfigDescriptor (tag 0x04): objectTypeIndication, the four
// fixed fields after it (streamType/upStream/reserved, bufferSizeDB,
// maxBitrate, avgBitrate - 13 bytes in all, never themselves read),
// then an optional nested DecoderSpecificInfo (tag 0x05).
func parseDecoderConfigDescriptor(body []byte) (objectTypeIndication byte, decoderSpecificInfo []byte, ok bool) {
	tag, length, consumed, ok := readMP4DescriptorHeader(body)
	if !ok || tag != 0x04 {
		return 0, nil, false
	}

	dcd := body[consumed : consumed+length]
	if len(dcd) < 13 {
		return 0, nil, false
	}
	objectTypeIndication = dcd[0]
	if len(dcd) == 13 {
		return objectTypeIndication, nil, true
	}

	tag2, length2, consumed2, ok := readMP4DescriptorHeader(dcd[13:])
	if !ok || tag2 != 0x05 {
		return 0, nil, false
	}
	return objectTypeIndication, dcd[13+consumed2 : 13+consumed2+length2], true
}

// readMP4DescriptorHeader reads one MPEG-4 descriptor's own tag and
// base-128 length (ISO/IEC 14496-1: up to 4 bytes, each contributing
// its own low 7 bits, continuing while the top bit is set) from the
// front of buf, returning the descriptor's own tag, declared body
// length and how many header bytes it took, bounded by buf's own
// length before anything past it is read.
func readMP4DescriptorHeader(buf []byte) (tag byte, length, consumed int, ok bool) {
	if len(buf) < 2 {
		return 0, 0, 0, false
	}
	tag = buf[0]

	pos := 1
	for i := 0; i < 4; i++ {
		if pos >= len(buf) {
			return 0, 0, 0, false
		}
		b := buf[pos]
		pos++
		length = length<<7 | int(b&0x7F)
		if b&0x80 == 0 {
			break
		}
		if i == 3 {
			return 0, 0, 0, false
		}
	}
	if length < 0 || pos+length > len(buf) {
		return 0, 0, 0, false
	}
	return tag, length, pos, true
}

// audioSpecificConfigAllowed reports whether asc's leading bits parse
// as an AudioSpecificConfig (ISO/IEC 14496-3) this package accepts:
// audioObjectType, samplingFrequencyIndex and channelConfiguration -
// the three fields FFmpeg's own AAC decoder reads to configure itself
// before any of asc's remaining, codec-specific bits matter at all.
func audioSpecificConfigAllowed(asc []byte) bool {
	if len(asc) < 2 {
		return false
	}
	if !allowedAACObjectType[asc[0]>>3] {
		return false
	}

	freqIndex := (asc[0]&0x07)<<1 | asc[1]>>7
	if freqIndex <= 12 {
		return channelConfigAllowed((asc[1] >> 3) & 0x0F)
	}
	return freqIndex == 15 && explicitAACSampleRateAllowed(asc)
}

// channelConfigAllowed reports whether cfg is one of
// AudioSpecificConfig's own defined, non-program-config-element
// channel layouts (1-7): 0 means channels are declared in a
// program_config_element this package never reads, and 8-15 are
// reserved.
func channelConfigAllowed(cfg byte) bool {
	return cfg >= 1 && cfg <= 7
}

// explicitAACSampleRateAllowed reads AudioSpecificConfig's own
// explicit 24-bit sample rate (used whenever samplingFrequencyIndex is
// 15, a rate outside the standard table) and the channel configuration
// right after it, bounded against asc's own length before either is
// read.
func explicitAACSampleRateAllowed(asc []byte) bool {
	if len(asc) < 5 {
		return false
	}
	rate := uint32(asc[1]&0x7F)<<17 | uint32(asc[2])<<9 | uint32(asc[3])<<1 | uint32(asc[4]>>7)
	if rate < 8000 || rate > 192_000 {
		return false
	}
	return channelConfigAllowed((asc[4] >> 3) & 0x0F)
}
