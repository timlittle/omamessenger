package audiosafe

// sampleentry.go judges what sits inside the one audio sample entry
// this package accepts, beyond its own codec fourcc: a security
// review found this package never checked an mp4a entry's own esds,
// or an Opus entry's own dOps, for anything at all - esds in
// particular can select any audio decoder FFmpeg supports regardless
// of the track's own stsd codec (see docs/decisions.md). This mirrors
// videosafe's own sampleentry.go (minus its avc1-specific checks,
// which this package has no equivalent of): the two packages judge
// different things and neither's cursor, span or box-reading helpers
// are exported, so this is a deliberate copy rather than a shared
// import (see isobmff.go's own doc comment).

// audioSampleEntry finds stbl's stsd box's one sample entry (already
// confirmed to be the only one stsd declares by sampleEntryCodec) and
// returns its own body: mirrors videosafe's own audioSampleEntry.
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
// own codec, and, for mp4a, that its one esds describes AAC: mirrors
// videosafe's own audioSampleEntryOK.
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
// describes AAC: mirrors videosafe's own mp4aSampleEntryOK.
func (c *cursor) mp4aSampleEntryOK(entry span) bool {
	esds, ok := c.findOneRequiredChild(entry, "esds", allowedMP4AChild)
	if !ok {
		return false
	}
	return c.esdsDescribesAAC(esds)
}

// opusSampleEntryOK reports whether entry carries exactly one dOps
// box among children that are all in allowedOpusChild, with its own
// declared version byte 0: mirrors videosafe's own opusSampleEntryOK.
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
// allowed, and returns the one child of type required: ok is true
// only when exactly one such child is present, however many of
// allowed's other box types sit alongside it. Mirrors videosafe's own
// findOneRequiredChild.
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
// a DecoderConfigDescriptor with objectTypeIndication 0x40 and a
// DecoderSpecificInfo this package accepts: mirrors videosafe's own
// esdsDescribesAAC.
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

// parseESDescriptor reads body as an MPEG-4 ES_Descriptor, assuming
// none of its own optional flag bits are set: mirrors videosafe's own
// parseESDescriptor.
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
// DecoderConfigDescriptor: mirrors videosafe's own
// parseDecoderConfigDescriptor.
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
// base-128 length from the front of buf: mirrors videosafe's own
// readMP4DescriptorHeader.
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
// as an AudioSpecificConfig this package accepts: mirrors videosafe's
// own audioSpecificConfigAllowed.
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
// channel layouts (1-7): mirrors videosafe's own
// channelConfigAllowed.
func channelConfigAllowed(cfg byte) bool {
	return cfg >= 1 && cfg <= 7
}

// explicitAACSampleRateAllowed reads AudioSpecificConfig's own
// explicit 24-bit sample rate and the channel configuration right
// after it: mirrors videosafe's own explicitAACSampleRateAllowed.
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
