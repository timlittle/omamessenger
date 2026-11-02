package videosafe

// sampletables.go closes a gap avcsamples.go only ever closed for an
// avc1 track's own stsz/stco/stsc layout: FFmpeg's own mov demuxer
// also sizes allocations from a track's stts, ctts, stss, sgpd and
// sbgp entry counts, and from its own trak-level edts/elst, for every
// track it opens, audio included. A security review found this
// package read an audio track's own sample count (sampleCount in
// isobmff.go) but never checked it against anything at all, and that
// none of these other tables were read for either kind of track: a
// voice note or audio-carrying video could declare a small, capped
// sample count in one table while a sibling table's own entry count
// or summed totals still implied a far larger one, exactly the shape
// FFmpeg's mov_build_index allocates sample_count*24 bytes from (see
// docs/decisions.md).

import "encoding/binary"

// sampleTablesConsistent reports whether stbl's own sample tables -
// stsz/stz2, stco/co64 and stsc (via sampleSpans, the same layout
// avcsamples.go's own in-band scan already relies on), stts, ctts,
// stss, sgpd and sbgp - all agree with each other and with
// sampleCount, for any track this package accepts, not only an avc1
// video track. sampleCount of 0 is vacuously consistent: there is
// nothing for any of these tables to agree on, and a track with no
// samples at all never reaches FFmpeg's own per-sample allocation
// path regardless of what else it declares.
func (c *cursor) sampleTablesConsistent(stbl span, sampleCount uint32) bool {
	if sampleCount == 0 {
		return true
	}
	if _, ok := c.sampleSpans(stbl, sampleCount); !ok {
		return false
	}
	if !c.sttsConsistent(stbl, sampleCount) {
		return false
	}
	if !c.cttsConsistent(stbl.off, stbl.len, sampleCount) {
		return false
	}
	if !c.stssConsistent(stbl.off, stbl.len, sampleCount) {
		return false
	}
	if !c.sbgpConsistent(stbl.off, stbl.len, sampleCount) {
		return false
	}
	return c.sgpdConsistent(stbl.off, stbl.len)
}

// sttsConsistent reports whether stbl's stts box - if present - sums
// to exactly sampleCount samples: the exact table a security review
// found FFmpeg's own mov_build_index allocates sample_count*24 bytes
// from, using the sum of stts's own per-entry sample_count fields
// rather than stsz's, so a track could declare a small, capped stsz
// count while a single stts entry still claimed tens of millions of
// samples. A track with no stts at all is accepted: sampleCount's own
// cap already bounds that case on its own.
func (c *cursor) sttsConsistent(stbl span, sampleCount uint32) bool {
	stts, found := c.firstChild(stbl.off, stbl.len, "stts")
	if !found {
		return true
	}
	total, ok := c.eightByteRunTotal(stts, maxSTTSEntries)
	return ok && total == uint64(sampleCount)
}

// cttsConsistent reports whether stbl's ctts box - if present - sums
// to exactly sampleCount samples, the same consistency sttsConsistent
// already applies to stts: ISO/IEC 14496-12 requires ctts, when
// present, to carry exactly one composition-offset entry per sample.
func (c *cursor) cttsConsistent(stblOff, stblLen int64, sampleCount uint32) bool {
	ctts, found := c.firstChild(stblOff, stblLen, "ctts")
	if !found {
		return true
	}
	total, ok := c.eightByteRunTotal(ctts, maxCttsEntries)
	return ok && total == uint64(sampleCount)
}

// eightByteRunTotal reads a box shaped like stts or ctts - a FullBox
// entry_count field at box.off+4, then that many 8-byte rows whose
// own first 4 bytes are a per-row sample count - summing every row's
// own sample count, bounded by maxEntries and the box's own declared
// length before any of it is allocated. ok is false once entry_count
// exceeds maxEntries or the rows would not fit within the box.
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
// declares no more entries than sampleCount, each fitting within the
// box's own declared length before any of it is allocated, and every
// sample number it names falls inside [1, sampleCount] in strictly
// increasing order - the order ISO/IEC 14496-12 itself requires.
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
// numbers are each within [1, sampleCount] and strictly increasing,
// split out of stssConsistent to keep its own branching within this
// package's complexity limit.
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
// to exactly sampleCount samples: ISO/IEC 14496-12 requires sbgp, when
// present, to map every sample to a group in contiguous runs covering
// the whole track, the same totals-must-agree rule already applied to
// stts and ctts above. Version 1 inserts a 4-byte
// grouping_type_parameter, between grouping_type and entry_count, that
// version 0 does not have.
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
// field, relative to sbgp.off: 8 for version 0, or 12 for version 1,
// once its own grouping_type_parameter is skipped.
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

// sbgpTotal reads sbgp's own entry_count field at sbgp.off+countOff
// and sums the sample_count half of each 8-byte (sample_count,
// group_description_index) row right after it, bounded by
// maxSbgpEntries and sbgp's own declared length before any of it is
// allocated.
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
// version 1 (the only version this package has ever seen a real
// encoder write, for AAC's own "roll" grouping type) with an
// entry_count under maxSampleGroupEntries, and whose own descriptions
// - each either default_length bytes, or its own 4-byte length prefix
// and that many bytes when default_length is 0 - stay within the
// box's own declared length throughout. This package never
// interprets a description's own payload: which group a sample
// belongs to never changes which bytes this package itself reads.
func (c *cursor) sgpdConsistent(stblOff, stblLen int64) bool {
	sgpd, found := c.firstChild(stblOff, stblLen, "sgpd")
	if !found {
		return true
	}
	if sgpd.len < 16 {
		return false
	}

	// version/flags(4) + grouping_type(4, never itself read - this
	// package never interprets a description's own payload either)
	// + default_length(4) + entry_count(4).
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

// skipSampleGroupDescription reads one sgpd description at off,
// bounded by end: defaultLength bytes when it is non-zero, or the
// description's own 4-byte big-endian length prefix and that many
// bytes when defaultLength is 0 (version 1's own variable-length
// shape), returning the offset right after it.
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
// carries no more than maxELSTEntries elst entries, and, when it
// carries exactly one, that entry's own segment duration - read
// through timescale as a conservative stand-in for the edit list's
// own movie-timescale units (ISO/IEC 14496-12 ties elst's own
// duration to mvhd's timescale; this package has no other reason to
// read mvhd's own raw timescale, so it bounds the value the same way
// this track's own duration is already bounded rather than plumb a
// second timescale through for one more field) - falls within
// [0, maxDuration]. A trak with no edts, or an edts with no elst, is
// accepted: neither declares anything this package needs to bound.
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
// exactly one, that one entry's own segment_duration (version 0's
// 4-byte field, or version 1's 8-byte one), reporting whether the
// duration it converts to through timescale falls within
// [0, maxDuration].
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
// one entry, right after its own 8-byte FullBox-and-entry_count
// header: 4 bytes for version 0, 8 for version 1 - ISO/IEC 14496-12's
// own two ElstEntry shapes.
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
