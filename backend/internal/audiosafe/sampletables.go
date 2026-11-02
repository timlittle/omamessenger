package audiosafe

// sampletables.go closes a gap this package never closed at all: a
// security review found it read the one audio track's own sample
// count but never checked it against anything else - not its own
// stsz/stco/stsc layout, nor its stts, ctts, stss, sgpd or sbgp
// tables, nor a trak-level edit list - every one of which FFmpeg's
// own mov demuxer sizes an allocation from (see docs/decisions.md).
// This mirrors videosafe's own sampletables.go and the sample-layout
// parts of its avcsamples.go: the two packages judge different
// things, and neither's cursor, span or box-reading helpers are
// exported, so this is a deliberate copy rather than a shared import
// (see isobmff.go's own doc comment).

import "encoding/binary"

// stscEntry is one row of stbl's stsc box: the first chunk (1-based)
// this entry applies to, how many samples every chunk from there up
// to the next entry's own first_chunk holds, and which sample
// description (1-based) those samples use.
type stscEntry struct {
	firstChunk             uint32
	samplesPerChunk        uint32
	sampleDescriptionIndex uint32
}

// sampleTablesConsistent reports whether stbl's own sample tables -
// stsz/stz2, stco/co64 and stsc (via sampleSpans), stts, ctts, stss,
// sgpd and sbgp - all agree with each other and with sampleCount.
// sampleCount of 0 is vacuously consistent: there is nothing for any
// of these tables to agree on.
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

// sampleSpans computes every sample's own offset and length from
// stbl's stsz (sample sizes), stco/co64 (chunk offsets) and stsc (how
// many samples each chunk holds), the same layout videosafe's own
// sampleSpans computes, used here only to confirm the three tables
// agree on a total sample count - this package never reads an actual
// sample's own bytes.
func (c *cursor) sampleSpans(stbl span, sampleCount uint32) ([]span, bool) {
	sizes, ok := c.sampleSizeTable(stbl.off, stbl.len, sampleCount)
	if !ok {
		return nil, false
	}
	offsets, ok := c.chunkOffsetTable(stbl.off, stbl.len)
	if !ok {
		return nil, false
	}
	entries, ok := c.sampleToChunkTable(stbl.off, stbl.len)
	if !ok {
		return nil, false
	}
	return layoutSamples(sizes, offsets, entries)
}

// layoutSamples places sizes (one per sample, in order) into chunks
// at chunkOffsets, exactly the way ISO/IEC 14496-12's stsc table
// assigns consecutive samples to consecutive chunks: mirrors
// videosafe's own layoutSamples.
func layoutSamples(sizes []uint32, chunkOffsets []int64, entries []stscEntry) ([]span, bool) {
	spans := make([]span, 0, len(sizes))
	sampleIdx := 0
	entryIdx := 0

	for chunkIdx, chunkOff := range chunkOffsets {
		if sampleIdx >= len(sizes) {
			break
		}

		chunkNum := uint32(chunkIdx + 1)
		for entryIdx+1 < len(entries) && entries[entryIdx+1].firstChunk <= chunkNum {
			entryIdx++
		}
		if entryIdx >= len(entries) {
			return nil, false
		}

		remaining := entries[entryIdx].samplesPerChunk
		off := chunkOff
		for i := uint32(0); i < remaining && sampleIdx < len(sizes); i++ {
			spans = append(spans, span{off: off, len: int64(sizes[sampleIdx])})
			off += int64(sizes[sampleIdx])
			sampleIdx++
		}
	}

	if sampleIdx != len(sizes) {
		return nil, false
	}
	return spans, true
}

// stscEntriesValid reports whether entries is a well-formed stsc
// table: mirrors videosafe's own stscEntriesValid.
func stscEntriesValid(entries []stscEntry) bool {
	if len(entries) == 0 {
		return true
	}
	if entries[0].firstChunk != 1 || entries[0].samplesPerChunk == 0 || entries[0].sampleDescriptionIndex != 1 {
		return false
	}
	for i := 1; i < len(entries); i++ {
		e, prev := entries[i], entries[i-1]
		if e.firstChunk <= prev.firstChunk || e.samplesPerChunk == 0 || e.sampleDescriptionIndex != 1 {
			return false
		}
	}
	return true
}

// sampleSizeTable reads stbl's stsz box for exactly sampleCount
// sample sizes: mirrors videosafe's own sampleSizeTable.
func (c *cursor) sampleSizeTable(stblOff, stblLen int64, sampleCount uint32) ([]uint32, bool) {
	stsz, found := c.firstChild(stblOff, stblLen, "stsz")
	if !found || stsz.len < 12 {
		return nil, false
	}

	var hdr [12]byte
	if !c.readAt(hdr[:], stsz.off) {
		return nil, false
	}
	if binary.BigEndian.Uint32(hdr[8:12]) != sampleCount {
		return nil, false
	}

	if fixedSize := binary.BigEndian.Uint32(hdr[4:8]); fixedSize != 0 {
		if int64(fixedSize)*int64(sampleCount) > c.size {
			return nil, false
		}
		return fixedSizeTable(fixedSize, sampleCount), true
	}
	return c.sampleSizeEntries(stsz, sampleCount)
}

// fixedSizeTable returns sampleCount copies of size: mirrors
// videosafe's own fixedSizeTable.
func fixedSizeTable(size, sampleCount uint32) []uint32 {
	sizes := make([]uint32, sampleCount)
	for i := range sizes {
		sizes[i] = size
	}
	return sizes
}

// sampleSizeEntries reads stsz's own per-sample table: mirrors
// videosafe's own sampleSizeEntries.
func (c *cursor) sampleSizeEntries(stsz span, sampleCount uint32) ([]uint32, bool) {
	need := int64(sampleCount) * 4
	if stsz.len < 12+need {
		return nil, false
	}

	buf := make([]byte, need)
	if !c.readAt(buf, stsz.off+12) {
		return nil, false
	}

	sizes := make([]uint32, sampleCount)
	for i := range sizes {
		sizes[i] = binary.BigEndian.Uint32(buf[i*4 : i*4+4])
	}
	return sizes, true
}

// chunkOffsetTable reads stbl's stco or co64 box for every chunk's
// own absolute file offset: mirrors videosafe's own chunkOffsetTable.
func (c *cursor) chunkOffsetTable(stblOff, stblLen int64) ([]int64, bool) {
	if stco, found := c.firstChild(stblOff, stblLen, "stco"); found {
		return c.offsetEntries(stco, 4)
	}
	if co64, found := c.firstChild(stblOff, stblLen, "co64"); found {
		return c.offsetEntries(co64, 8)
	}
	return nil, false
}

// offsetEntries reads box's own entry_count field and that many
// width-byte (4 or 8) big-endian offsets right after it: mirrors
// videosafe's own offsetEntries.
func (c *cursor) offsetEntries(box span, width int64) ([]int64, bool) {
	if box.len < 8 {
		return nil, false
	}

	var countBuf [4]byte
	if !c.readAt(countBuf[:], box.off+4) {
		return nil, false
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count > maxChunkCount {
		return nil, false
	}

	need := int64(count) * width
	if box.len < 8+need {
		return nil, false
	}

	buf := make([]byte, need)
	if !c.readAt(buf, box.off+8) {
		return nil, false
	}

	offsets := make([]int64, count)
	for i := range offsets {
		offsets[i] = readFileOffset(buf[int64(i)*width:int64(i)*width+width], width)
	}
	return offsets, true
}

// readFileOffset reads width bytes (4 or 8) from the start of buf as
// a big-endian unsigned integer: mirrors videosafe's own
// readFileOffset.
func readFileOffset(buf []byte, width int64) int64 {
	if width == 8 {
		return int64(binary.BigEndian.Uint64(buf))
	}
	return int64(binary.BigEndian.Uint32(buf))
}

// sampleToChunkTable reads stbl's stsc box for every entry: mirrors
// videosafe's own sampleToChunkTable.
func (c *cursor) sampleToChunkTable(stblOff, stblLen int64) ([]stscEntry, bool) {
	stsc, found := c.firstChild(stblOff, stblLen, "stsc")
	if !found || stsc.len < 8 {
		return nil, false
	}

	var countBuf [4]byte
	if !c.readAt(countBuf[:], stsc.off+4) {
		return nil, false
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count > maxStscEntries {
		return nil, false
	}

	need := int64(count) * 12
	if stsc.len < 8+need {
		return nil, false
	}

	buf := make([]byte, need)
	if !c.readAt(buf, stsc.off+8) {
		return nil, false
	}

	entries := make([]stscEntry, count)
	for i := range entries {
		row := buf[int64(i)*12:]
		entries[i] = stscEntry{
			firstChunk:             binary.BigEndian.Uint32(row[0:4]),
			samplesPerChunk:        binary.BigEndian.Uint32(row[4:8]),
			sampleDescriptionIndex: binary.BigEndian.Uint32(row[8:12]),
		}
	}
	if !stscEntriesValid(entries) {
		return nil, false
	}
	return entries, true
}
