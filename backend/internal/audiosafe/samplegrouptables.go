package audiosafe

// samplegrouptables.go covers the sample-table checks sampleTablesConsistent
// (sampletables.go) calls beyond the stsz/stco/stsc layout triangle:
// stts, ctts, stss, sgpd and sbgp, plus a trak's own edit list - split
// out once sampletables.go grew past this package's own file-length
// limit. Mirrors videosafe's own sampletables.go (the same split that
// file never needed, since it shares a file with isobmff.go's own
// box walker instead).

import "encoding/binary"

// sttsConsistent reports whether stbl's stts box - if present - sums
// to exactly sampleCount samples: mirrors videosafe's own
// sttsConsistent.
func (c *cursor) sttsConsistent(stbl span, sampleCount uint32) bool {
	stts, found := c.firstChild(stbl.off, stbl.len, "stts")
	if !found {
		return true
	}
	total, ok := c.eightByteRunTotal(stts, maxSTTSEntries)
	return ok && total == uint64(sampleCount)
}

// cttsConsistent reports whether stbl's ctts box - if present - sums
// to exactly sampleCount samples: mirrors videosafe's own
// cttsConsistent.
func (c *cursor) cttsConsistent(stblOff, stblLen int64, sampleCount uint32) bool {
	ctts, found := c.firstChild(stblOff, stblLen, "ctts")
	if !found {
		return true
	}
	total, ok := c.eightByteRunTotal(ctts, maxCttsEntries)
	return ok && total == uint64(sampleCount)
}

// eightByteRunTotal reads a box shaped like stts or ctts: mirrors
// videosafe's own eightByteRunTotal.
func (c *cursor) eightByteRunTotal(box span, maxEntries uint32) (total uint64, ok bool) {
	if box.len < 8 {
		return 0, false
	}
	var countBuf [4]byte
	if !c.readAt(countBuf[:], box.off+4) {
		return 0, false
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count > maxEntries {
		return 0, false
	}

	need := int64(count) * 8
	if box.len < 8+need {
		return 0, false
	}
	buf := make([]byte, need)
	if !c.readAt(buf, box.off+8) {
		return 0, false
	}

	for i := uint32(0); i < count; i++ {
		total += uint64(binary.BigEndian.Uint32(buf[i*8 : i*8+4]))
	}
	return total, true
}

// stssConsistent reports whether stbl's stss box - if present -
// declares no more entries than sampleCount, each within
// [1, sampleCount] in strictly increasing order: mirrors videosafe's
// own stssConsistent.
func (c *cursor) stssConsistent(stblOff, stblLen int64, sampleCount uint32) bool {
	stss, found := c.firstChild(stblOff, stblLen, "stss")
	if !found {
		return true
	}
	if stss.len < 8 {
		return false
	}

	var countBuf [4]byte
	if !c.readAt(countBuf[:], stss.off+4) {
		return false
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count > maxStssEntries || count > sampleCount {
		return false
	}

	need := int64(count) * 4
	if stss.len < 8+need {
		return false
	}
	buf := make([]byte, need)
	if !c.readAt(buf, stss.off+8) {
		return false
	}
	return stssEntriesInOrder(buf, count, sampleCount)
}

// stssEntriesInOrder reports whether buf's count 4-byte sample
// numbers are each within [1, sampleCount] and strictly increasing:
// mirrors videosafe's own stssEntriesInOrder.
func stssEntriesInOrder(buf []byte, count, sampleCount uint32) bool {
	var prev uint32
	for i := uint32(0); i < count; i++ {
		v := binary.BigEndian.Uint32(buf[i*4 : i*4+4])
		if v < 1 || v > sampleCount || v <= prev {
			return false
		}
		prev = v
	}
	return true
}

// sbgpConsistent reports whether stbl's sbgp box - if present - sums
// to exactly sampleCount samples: mirrors videosafe's own
// sbgpConsistent.
func (c *cursor) sbgpConsistent(stblOff, stblLen int64, sampleCount uint32) bool {
	sbgp, found := c.firstChild(stblOff, stblLen, "sbgp")
	if !found {
		return true
	}
	countOff, ok := c.sbgpCountOffset(sbgp)
	if !ok {
		return false
	}
	total, ok := c.sbgpTotal(sbgp, countOff)
	return ok && total == uint64(sampleCount)
}

// sbgpCountOffset reports the byte offset of sbgp's own entry_count
// field, relative to sbgp.off: mirrors videosafe's own
// sbgpCountOffset.
func (c *cursor) sbgpCountOffset(sbgp span) (int64, bool) {
	if sbgp.len < 8 {
		return 0, false
	}
	var version [1]byte
	if !c.readAt(version[:], sbgp.off) {
		return 0, false
	}
	if version[0] == 1 {
		return 12, true
	}
	return 8, true
}

// sbgpTotal reads sbgp's own entry_count field and sums the
// sample_count half of each row right after it: mirrors videosafe's
// own sbgpTotal.
func (c *cursor) sbgpTotal(sbgp span, countOff int64) (uint64, bool) {
	if sbgp.len < countOff+4 {
		return 0, false
	}
	var countBuf [4]byte
	if !c.readAt(countBuf[:], sbgp.off+countOff) {
		return 0, false
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count > maxSbgpEntries {
		return 0, false
	}

	need := int64(count) * 8
	if sbgp.len < countOff+4+need {
		return 0, false
	}
	buf := make([]byte, need)
	if !c.readAt(buf, sbgp.off+countOff+4) {
		return 0, false
	}

	var total uint64
	for i := uint32(0); i < count; i++ {
		total += uint64(binary.BigEndian.Uint32(buf[i*8 : i*8+4]))
	}
	return total, true
}

// sgpdConsistent reports whether stbl's sgpd box - if present - is
// version 1 with an entry_count under maxSampleGroupEntries, and
// whose own descriptions stay within the box's own declared length
// throughout: mirrors videosafe's own sgpdConsistent.
func (c *cursor) sgpdConsistent(stblOff, stblLen int64) bool {
	sgpd, found := c.firstChild(stblOff, stblLen, "sgpd")
	if !found {
		return true
	}
	if sgpd.len < 16 {
		return false
	}

	// version/flags(4) + grouping_type(4, never itself read) +
	// default_length(4) + entry_count(4).
	var hdr [16]byte
	if !c.readAt(hdr[:], sgpd.off) {
		return false
	}
	if hdr[0] != 1 {
		return false
	}

	defaultLength := binary.BigEndian.Uint32(hdr[8:12])
	count := binary.BigEndian.Uint32(hdr[12:16])
	if count > maxSampleGroupEntries {
		return false
	}

	end := sgpd.off + sgpd.len
	off := sgpd.off + 16
	for i := uint32(0); i < count; i++ {
		var ok bool
		off, ok = c.skipSampleGroupDescription(off, end, defaultLength)
		if !ok {
			return false
		}
	}
	return true
}

// skipSampleGroupDescription reads one sgpd description at off:
// mirrors videosafe's own skipSampleGroupDescription.
func (c *cursor) skipSampleGroupDescription(off, end int64, defaultLength uint32) (int64, bool) {
	if defaultLength != 0 {
		next := off + int64(defaultLength)
		if next > end {
			return 0, false
		}
		return next, true
	}

	if off+4 > end {
		return 0, false
	}
	var lenBuf [4]byte
	if !c.readAt(lenBuf[:], off) {
		return 0, false
	}
	n := int64(binary.BigEndian.Uint32(lenBuf[:]))
	next := off + 4 + n
	if n < 0 || next > end {
		return 0, false
	}
	return next, true
}

// editListAllowed reports whether trak's own edts box - if present -
// carries no more than maxELSTEntries elst entries, in range: mirrors
// videosafe's own editListAllowed.
func (c *cursor) editListAllowed(trakOff, trakLen int64, timescale uint32) bool {
	edts, found := c.firstChild(trakOff, trakLen, "edts")
	if !found {
		return true
	}
	elst, found := c.firstChild(edts.off, edts.len, "elst")
	if !found {
		return true
	}
	return c.elstEntryInRange(elst, timescale)
}

// elstEntryInRange reads elst's own entry_count and, when it is
// exactly one, that one entry's own segment_duration: mirrors
// videosafe's own elstEntryInRange.
func (c *cursor) elstEntryInRange(elst span, timescale uint32) bool {
	if elst.len < 8 {
		return false
	}
	var hdr [8]byte
	if !c.readAt(hdr[:], elst.off) {
		return false
	}

	version := hdr[0]
	count := binary.BigEndian.Uint32(hdr[4:8])
	if count > maxELSTEntries {
		return false
	}
	if count == 0 {
		return true
	}

	segmentDuration, ok := c.elstSegmentDuration(elst, version)
	if !ok {
		return false
	}
	return durationSeconds(timescale, segmentDuration) <= maxDuration.Seconds()
}

// elstSegmentDuration reads the raw segment_duration field of elst's
// one entry: mirrors videosafe's own elstSegmentDuration.
func (c *cursor) elstSegmentDuration(elst span, version byte) (uint64, bool) {
	if version == 1 {
		if elst.len < 16 {
			return 0, false
		}
		var buf [8]byte
		if !c.readAt(buf[:], elst.off+8) {
			return 0, false
		}
		return binary.BigEndian.Uint64(buf[:]), true
	}

	if elst.len < 12 {
		return 0, false
	}
	var buf [4]byte
	if !c.readAt(buf[:], elst.off+8) {
		return 0, false
	}
	return uint64(binary.BigEndian.Uint32(buf[:])), true
}
