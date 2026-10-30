package videosafe

// avcsamples.go closes the one gap avcC's own SPS cannot: an avc1
// sample's compressed data is itself a run of length-prefixed NAL
// units, and FFmpeg's H.264 decoder reads any sequence or picture
// parameter set it finds there, in-band, exactly as if it had come
// from avcC. A sender can leave a small, "safe-looking" avcC in
// place and still smuggle a second, much larger SPS inside a sample,
// which the decoder uses once a later slice refers to it (see
// docs/decisions.md). This file walks every sample's own bytes - the
// one kind of sample data this package otherwise never reads - far
// enough to refuse that, and nothing else: it never decodes a frame,
// only the length prefix and NAL header byte in front of each one,
// plus the handful of bytes an in-band SPS or PPS itself needs to be
// judged.
//
// A sample's own byte range in the file is computed the same way
// ISO/IEC 14496-12's mov demuxer finds it: stsz (or a fixed sample
// size) for each sample's own length, stco/co64 for every chunk's
// file offset, and stsc for how many consecutive samples each chunk
// holds. Any of this that cannot be read in full - including stz2's
// own variable-width table, which this package does not support -
// refuses the track outright, the same "cannot read it, cannot trust
// it" rule every other check in this package already applies. The
// running total of every sample's own length is also bounded against
// the file's own size as the scan proceeds, so a crafted layout that
// revisits the same bytes many times can never make this cost more
// than reading the file once.

import (
	"bytes"
	"encoding/binary"
)

// avcSamplesSafe reports whether every one of an avc1 track's own
// samples is free of an in-band parameter set FFmpeg would still
// honor. A track with no samples at all is vacuously safe: there is
// nothing to scan. sampleCount above maxSampleCount, or a track whose
// sample layout cannot be fully determined from stsz, stco/co64 and
// stsc, is refused rather than skipped.
func (c *cursor) avcSamplesSafe(stbl span, sampleCount uint32, cfg avcDecoderConfig) bool {
	if sampleCount == 0 {
		return true
	}
	if sampleCount > maxSampleCount {
		return false
	}

	samples, ok := c.sampleSpans(stbl, sampleCount)
	if !ok {
		return false
	}

	var totalBytes int64
	for _, sample := range samples {
		totalBytes += sample.len
		if totalBytes < 0 || totalBytes > c.size {
			return false
		}
		if !c.scanAVCSample(sample, cfg) {
			return false
		}
	}
	return true
}

// scanAVCSample reads one sample's full bytes - bounded against the
// file's own size before anything is allocated, the same bound
// sampleSpans already placed each sample's own offset and length
// within - and walks its length-prefixed NAL units in memory,
// refusing it outright at the first one avcNALAllowed refuses.
func (c *cursor) scanAVCSample(sample span, cfg avcDecoderConfig) bool {
	if sample.off < 0 || sample.len < 0 || sample.len > c.size-sample.off {
		return false
	}

	buf := make([]byte, sample.len)
	if !c.readAt(buf, sample.off) {
		return false
	}

	for len(buf) > 0 {
		if int64(len(buf)) < int64(cfg.lengthSize) {
			return false
		}
		nalLen := readNALLength(buf[:cfg.lengthSize])
		buf = buf[cfg.lengthSize:]
		if nalLen < 0 || nalLen > int64(len(buf)) {
			return false
		}
		if !avcNALAllowed(buf[:nalLen], cfg) {
			return false
		}
		buf = buf[nalLen:]
	}
	return true
}

// readNALLength decodes buf (1 to 4 bytes, avcC's own
// lengthSizeMinusOne+1) as a big-endian NAL length: always small
// enough to fit an int64 without any risk of overflow.
func readNALLength(buf []byte) int64 {
	var v uint32
	for _, b := range buf {
		v = v<<8 | uint32(b)
	}
	return int64(v)
}

// avcNALAllowed reports whether nal - one sample's own NAL unit, NAL
// header byte included, length prefix already stripped - is safe to
// leave for FFmpeg to decode. An in-band sequence parameter set (NAL
// type 7) is allowed only when it is byte for byte identical to
// cfg's own trusted avcC SPS - a real encoder has no reason to repeat
// an SPS in-band unless it is exactly the one already declared, so
// this is a safe, documented exception rather than a weakening (see
// docs/decisions.md). A sequence parameter set extension or subset
// SPS (types 13 and 15) has no avcC equivalent to ever match, so both
// are always refused. A picture parameter set (type 8) is refused
// once it names a seq_parameter_set_id other than cfg's own: this
// package only ever trusts the one SPS id it already validated.
// Every other NAL type passes through unjudged: this package has no
// opinion on ordinary slice data.
func avcNALAllowed(nal []byte, cfg avcDecoderConfig) bool {
	if len(nal) == 0 {
		return false
	}

	switch nal[0] & 0x1f {
	case 7:
		return len(nal) <= maxSPSLength && bytes.Equal(nal, cfg.sps)
	case 13, 15:
		return false
	case 8:
		return len(nal) <= maxPPSLength && ppsReferencesSPS(nal, cfg.spsID)
	default:
		return true
	}
}

// ppsReferencesSPS reads nal (a PPS NAL unit, header byte included)
// far enough to decode its own seq_parameter_set_id - the second
// Exp-Golomb field, right after pic_parameter_set_id - and reports
// whether it equals spsID. A PPS this package cannot read that far
// into is refused the same way an unparseable SPS already is.
func ppsReferencesSPS(nal []byte, spsID uint32) bool {
	r := &bitReader{data: removeEmulationPrevention(nal[1:])}

	if _, ok := r.ue(); !ok { // pic_parameter_set_id
		return false
	}
	seqParameterSetID, ok := r.ue()
	return ok && seqParameterSetID == spsID
}

// stscEntry is one row of stbl's stsc box: the first chunk (1-based)
// this entry applies to, and how many samples every chunk from there
// up to the next entry's own first_chunk holds.
type stscEntry struct {
	firstChunk      uint32
	samplesPerChunk uint32
}

// sampleSpans computes every sample's own offset and length in the
// file, from stbl's stsz (sample sizes), stco/co64 (chunk offsets)
// and stsc (how many samples each chunk holds) - the layout FFmpeg's
// own mov demuxer computes to find each sample's bytes, which this
// package reconstructs the same way to scan the right ones. ok is
// false whenever any of those three is missing, or stz2's own
// variable-width table is in play instead of stsz: see this file's
// own doc comment.
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
// assigns consecutive samples to consecutive chunks. ok is false once
// the tables disagree on how many samples exist: every chunk used up
// before every size is placed, or the reverse.
func layoutSamples(sizes []uint32, chunkOffsets []int64, entries []stscEntry) ([]span, bool) {
	spans := make([]span, 0, len(sizes))
	sampleIdx := 0

	for chunkIdx, chunkOff := range chunkOffsets {
		if sampleIdx >= len(sizes) {
			break
		}

		remaining := samplesInChunk(entries, uint32(chunkIdx+1))
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

// samplesInChunk returns the sample count stsc's own entries assign
// to chunkNum (1-based): the last entry whose own first_chunk is at
// or before chunkNum, exactly how ISO/IEC 14496-12 defines the table.
func samplesInChunk(entries []stscEntry, chunkNum uint32) uint32 {
	var count uint32
	for _, e := range entries {
		if e.firstChunk > chunkNum {
			break
		}
		count = e.samplesPerChunk
	}
	return count
}

// sampleSizeTable reads stbl's stsz box for exactly sampleCount
// sample sizes: either one fixed size repeated (stsz's own
// sample_size field, when non-zero) or that many individual 32-bit
// entries. ok is false once stsz's own declared sample_count
// disagrees with sampleCount, or it is missing entirely - including
// when the track uses stz2's own variable-width table instead, which
// this package does not support.
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
		return fixedSizeTable(fixedSize, sampleCount), true
	}
	return c.sampleSizeEntries(stsz, sampleCount)
}

// fixedSizeTable returns sampleCount copies of size: stsz's own way
// of saying every sample in the track is exactly that many bytes.
func fixedSizeTable(size, sampleCount uint32) []uint32 {
	sizes := make([]uint32, sampleCount)
	for i := range sizes {
		sizes[i] = size
	}
	return sizes
}

// sampleSizeEntries reads stsz's own per-sample table: sampleCount
// 32-bit entries right after its fixed 12-byte header, bounded by the
// box's own declared length before any of it is allocated.
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
// own absolute file offset: stco's 32-bit entries for an ordinary
// file, co64's 64-bit entries for one whose chunks live past the
// 32-bit range. ok is false once neither box is present.
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
// width-byte (4 or 8) big-endian offsets right after it, bounded by
// the box's own declared length before any of it is allocated.
func (c *cursor) offsetEntries(box span, width int64) ([]int64, bool) {
	if box.len < 8 {
		return nil, false
	}

	var countBuf [4]byte
	if !c.readAt(countBuf[:], box.off+4) {
		return nil, false
	}
	count := binary.BigEndian.Uint32(countBuf[:])

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
// a big-endian unsigned integer. A 64-bit value large enough to turn
// negative once converted to int64 is never a real file offset, and
// every later bounds check that compares an offset against the
// file's own size already treats a negative one as out of range.
func readFileOffset(buf []byte, width int64) int64 {
	if width == 8 {
		return int64(binary.BigEndian.Uint64(buf))
	}
	return int64(binary.BigEndian.Uint32(buf))
}

// sampleToChunkTable reads stbl's stsc box for every entry: its own
// entry_count field, then that many 12-byte rows (first_chunk,
// samples_per_chunk, sample_description_index - the third field this
// package never needs), bounded by the box's own declared length
// before any of it is allocated.
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
			firstChunk:      binary.BigEndian.Uint32(row[0:4]),
			samplesPerChunk: binary.BigEndian.Uint32(row[4:8]),
		}
	}
	return entries, true
}
