package videosafe

// avcsamples_internal_test.go reaches avcsamples.go's own defensive
// branches the same way readfailure_internal_test.go reaches
// isobmff.go's: a box too short to hold the fixed header its own
// reader needs, a declared count too large for what the box can
// actually hold, a declared sample_count that disagrees with what the
// caller already read elsewhere, and a reader that fails on demand -
// none of which a well-formed file, or the public Validate(path) API
// backed by a real *os.File, can ever reach on its own. It reuses
// endFailingReaderAt, box and sampleSize from readfailure_internal_test.go
// rather than redefining them.

import (
	"bytes"
	"context"
	"encoding/binary"
	"runtime"
	"testing"
	"time"
)

// TestSampleSizeTable_EdgeCases covers every way sampleSizeTable
// itself can refuse a track: no stsz box, one too short to hold its
// own fixed header, a declared sample_count that disagrees with the
// caller's own count, that fixed header's read failing, a per-sample
// table shorter than its own declared count, and that table's own
// bulk read failing.
func TestSampleSizeTable_EdgeCases(t *testing.T) {
	t.Parallel()

	run := func(name string, data []byte, failPast int64, sampleCount uint32) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: &endFailingReaderAt{data: data, failPast: failPast}, size: int64(len(data)), boxes: maxBoxBudget}
			if _, ok := c.sampleSizeTable(0, int64(len(data)), sampleCount); ok {
				t.Errorf("sampleSizeTable() succeeded for %q, want refused", name)
			}
		})
	}

	noStsz := box("stbl", []byte{})
	run("no stsz box at all", noStsz, int64(len(noStsz)), 1)

	tooShort := box("stsz", make([]byte, 8))
	run("stsz too short for its own fixed header", tooShort, int64(len(tooShort)), 1)

	mismatched := box("stsz", sampleSize(5))
	run("declared sample_count disagrees with the caller's own count", mismatched, int64(len(mismatched)), 6)

	headerReadFails := box("stsz", sampleSize(1))
	run("fixed header read fails", headerReadFails, 8, 1)

	tableTooShort := box("stsz", sampleSize(100)) // sample_count 100, no entries follow
	run("per-sample table shorter than its own declared count", tableTooShort, int64(len(tableTooShort)), 100)

	tableReadFails := box("stsz", append(sampleSize(3), make([]byte, 12)...))
	run("per-sample table bulk read fails", tableReadFails, 20, 3)
}

// TestOffsetEntries_EdgeCases covers every way chunkOffsetTable's own
// offsetEntries can refuse a track: a box too short to hold its own
// entry_count field, that field's read failing, a declared count
// too large for the box's own length, and the entries' own bulk read
// failing - once for stco's 32-bit entries and once for co64's own
// 64-bit width.
func TestOffsetEntries_EdgeCases(t *testing.T) {
	t.Parallel()

	run := func(name, boxType string, data []byte, failPast int64) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: &endFailingReaderAt{data: data, failPast: failPast}, size: int64(len(data)), boxes: maxBoxBudget}
			if _, ok := c.chunkOffsetTable(0, int64(len(data))); ok {
				t.Errorf("chunkOffsetTable() succeeded for %q (%s), want refused", name, boxType)
			}
		})
	}

	tooShort := box("stco", make([]byte, 4))
	run("stco too short for its own entry_count field", "stco", tooShort, int64(len(tooShort)))

	// failPast 12 lets the box's own 8-byte header read (ending at 8)
	// succeed, then fails the entry_count field read (box.off+4,
	// ending at 16).
	countReadFails := box("stco", make([]byte, 8))
	run("entry_count read fails", "stco", countReadFails, 12)

	entryCount := make([]byte, 8)
	entryCount[7] = 5 // entry_count = 5, no entries follow
	tableTooShort := box("stco", entryCount)
	run("declared count too large for the box itself", "stco", tableTooShort, int64(len(tableTooShort)))

	// failPast 16 lets the header and entry_count field (ending at 16)
	// succeed, then fails the entries' own bulk read.
	entriesReadFails := box("stco", append(func() []byte { b := make([]byte, 8); b[7] = 2; return b }(), make([]byte, 8)...))
	run("entries bulk read fails", "stco", entriesReadFails, 16)

	co64TooShort := box("co64", make([]byte, 4))
	run("co64 too short for its own entry_count field", "co64", co64TooShort, int64(len(co64TooShort)))

	co64EntriesReadFails := box("co64", append(func() []byte { b := make([]byte, 8); b[7] = 1; return b }(), make([]byte, 8)...))
	run("co64 entries bulk read fails", "co64", co64EntriesReadFails, 16)
}

// TestSampleToChunkTable_EdgeCases covers every way
// sampleToChunkTable can refuse a track: no stsc box, one too short
// to hold its own entry_count field, that field's read failing, a
// declared count too large for the box's own length, and the
// entries' own bulk read failing.
func TestSampleToChunkTable_EdgeCases(t *testing.T) {
	t.Parallel()

	run := func(name string, data []byte, failPast int64) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: &endFailingReaderAt{data: data, failPast: failPast}, size: int64(len(data)), boxes: maxBoxBudget}
			if _, ok := c.sampleToChunkTable(0, int64(len(data))); ok {
				t.Errorf("sampleToChunkTable() succeeded for %q, want refused", name)
			}
		})
	}

	noStsc := box("stbl", []byte{})
	run("no stsc box at all", noStsc, int64(len(noStsc)))

	tooShort := box("stsc", make([]byte, 4))
	run("stsc too short for its own entry_count field", tooShort, int64(len(tooShort)))

	// failPast 12 lets the box's own 8-byte header read (ending at 8)
	// succeed, then fails the entry_count field read (ending at 16).
	countReadFails := box("stsc", make([]byte, 8))
	run("entry_count read fails", countReadFails, 12)

	entryCount := make([]byte, 8)
	entryCount[7] = 5 // entry_count = 5, no rows follow
	tableTooShort := box("stsc", entryCount)
	run("declared count too large for the box itself", tableTooShort, int64(len(tableTooShort)))

	// failPast 16 lets the header and entry_count field (ending at 16)
	// succeed, then fails the rows' own bulk read.
	entriesHeader := make([]byte, 8)
	entriesHeader[7] = 1 // declares one stsc entry
	entriesReadFails := box("stsc", append(entriesHeader, make([]byte, 12)...))
	run("entries bulk read fails", entriesReadFails, 16)
}

// TestScanAVCSample_ReadFails covers scanAVCSample's own bounded read
// of a sample's full bytes failing after the bound check it is
// guarded by already passed - the one branch no well-formed file,
// where a found span is always backed by real bytes, can ever reach.
func TestScanAVCSample_ReadFails(t *testing.T) {
	t.Parallel()

	data := make([]byte, 16)
	c := &cursor{f: &endFailingReaderAt{data: data, failPast: 4}, size: int64(len(data)), boxes: maxBoxBudget}

	cfg := avcDecoderConfig{lengthSize: 4}
	if c.scanAVCSample(span{off: 0, len: 8}, cfg) {
		t.Error("scanAVCSample() succeeded once its own read failed, want refused")
	}
}

// fixedSampleSize builds a minimal stsz body declaring a fixed sample
// size (unlike readfailure_internal_test.go's own sampleSize, whose
// sample_size field is always 0, for the per-sample table instead).
func fixedSampleSize(size, sampleCount uint32) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[4:8], size)
	binary.BigEndian.PutUint32(buf[8:12], sampleCount)
	return buf
}

// TestSampleSizeTable_RefusesFixedSizeBeyondFileSize covers the fix
// for a tiny file whose stsz declares a fixed sample size and a huge
// sample count: fixedSize and sampleCount are each just one field, so
// nothing about a well-formed stsz box otherwise forces the resulting
// table to cost anything proportional to the file's own real size -
// unlike the per-sample table TestSampleSizeTable_EdgeCases already
// covers, which cannot declare more entries than its own box has
// bytes for. sampleSizeTable must refuse this before fixedSizeTable
// ever allocates sampleCount samples' worth of sizes, which this
// proves with a byte budget rather than timing: AllocsPerRun only
// counts allocations, and one giant slice counts the same as one tiny
// one.
func TestSampleSizeTable_RefusesFixedSizeBeyondFileSize(t *testing.T) {
	data := box("stsz", fixedSampleSize(1, maxSampleCount)) // 1 byte * 10,000,000 samples, from a file of a few dozen bytes
	c := &cursor{f: bytes.NewReader(data), size: int64(len(data)), boxes: maxBoxBudget}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, ok := c.sampleSizeTable(0, int64(len(data)), maxSampleCount)
	runtime.ReadMemStats(&after)

	if ok {
		t.Error("sampleSizeTable() succeeded for a fixed size whose total exceeds the file's own size, want refused")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Errorf("sampleSizeTable() allocated %d bytes refusing an infeasible fixed-size table, want well under 1 MiB: it must refuse before fixedSizeTable ever builds the table", allocated)
	}
}

// TestChunkOffsetTable_RefusesCountAboveLimit and
// TestSampleToChunkTable_RefusesCountAboveLimit below give their box a
// declared size (the box's own leading 4 bytes), far larger than the
// tiny real array backing it, and a correspondingly large container
// window, so the ordinary "box too short for its declared count"
// check cannot be what refuses them - isolating
// maxAVCChunkCount/maxAVCStscEntries as the actual reason. That is
// proved the same way as the fixed-size case above: by the allocation
// that refusing early avoids, not by the boolean result alone, which
// a real file this tiny would already fail on its own insufficient
// bytes either way.

// TestChunkOffsetTable_RefusesCountAboveLimit covers the cap on how
// many chunk offsets (stco entries) avcsamples.go will ever lay out a
// track's samples across.
func TestChunkOffsetTable_RefusesCountAboveLimit(t *testing.T) {
	count := uint32(maxAVCChunkCount + 1)
	data := make([]byte, 16)
	binary.BigEndian.PutUint32(data[0:4], 0x7FFFFFFF) // the box's own declared size, far larger than this real, tiny array
	copy(data[4:8], "stco")
	binary.BigEndian.PutUint32(data[12:16], count) // entry_count, at box.off+4 (box.off is 8, right after the type)

	c := &cursor{f: bytes.NewReader(data), size: int64(len(data)), boxes: maxBoxBudget}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, ok := c.chunkOffsetTable(0, 1<<40)
	runtime.ReadMemStats(&after)

	if ok {
		t.Error("chunkOffsetTable() succeeded for a count above maxAVCChunkCount, want refused")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Errorf("chunkOffsetTable() allocated %d bytes refusing a count above maxAVCChunkCount, want well under 1 MiB: it must refuse before building the offsets table", allocated)
	}
}

// TestSampleToChunkTable_RefusesCountAboveLimit covers the cap on how
// many sample-to-chunk rows (stsc entries) avcsamples.go will ever
// read, mirroring TestChunkOffsetTable_RefusesCountAboveLimit.
func TestSampleToChunkTable_RefusesCountAboveLimit(t *testing.T) {
	count := uint32(maxAVCStscEntries + 1)
	data := make([]byte, 16)
	binary.BigEndian.PutUint32(data[0:4], 0x7FFFFFFF)
	copy(data[4:8], "stsc")
	binary.BigEndian.PutUint32(data[12:16], count)

	c := &cursor{f: bytes.NewReader(data), size: int64(len(data)), boxes: maxBoxBudget}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, ok := c.sampleToChunkTable(0, 1<<40)
	runtime.ReadMemStats(&after)

	if ok {
		t.Error("sampleToChunkTable() succeeded for a count above maxAVCStscEntries, want refused")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Errorf("sampleToChunkTable() allocated %d bytes refusing a count above maxAVCStscEntries, want well under 1 MiB: it must refuse before building the entries table", allocated)
	}
}

// TestStscEntriesValid covers every shape stscEntriesValid must judge:
// empty (vacuously valid), starting at chunk 1 with strictly
// increasing first_chunk values and every samples_per_chunk above
// zero (valid), and each way a crafted table can depart from that -
// out of order, duplicated, not starting at chunk 1, or a zero
// samples_per_chunk row - which the old algorithm's unbounded rescan
// let through and an attacker used to defeat its own early-break
// optimization (see avcsamples.go's own doc comment).
func TestStscEntriesValid(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		entries []stscEntry
		want    bool
	}{
		"empty":              {nil, true},
		"single valid entry": {[]stscEntry{{firstChunk: 1, samplesPerChunk: 1}}, true},
		"several valid entries": {
			[]stscEntry{{firstChunk: 1, samplesPerChunk: 2}, {firstChunk: 3, samplesPerChunk: 1}}, true,
		},
		"does not start at chunk 1": {[]stscEntry{{firstChunk: 2, samplesPerChunk: 1}}, false},
		"out of order": {
			[]stscEntry{{firstChunk: 1, samplesPerChunk: 1}, {firstChunk: 3, samplesPerChunk: 1}, {firstChunk: 2, samplesPerChunk: 1}}, false,
		},
		"duplicate first_chunk": {
			[]stscEntry{{firstChunk: 1, samplesPerChunk: 1}, {firstChunk: 1, samplesPerChunk: 1}}, false,
		},
		"zero samples_per_chunk": {[]stscEntry{{firstChunk: 1, samplesPerChunk: 0}}, false},
		"zero samples_per_chunk, not first": {
			[]stscEntry{{firstChunk: 1, samplesPerChunk: 1}, {firstChunk: 2, samplesPerChunk: 0}}, false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := stscEntriesValid(tc.entries); got != tc.want {
				t.Errorf("stscEntriesValid(%+v) = %t, want %t", tc.entries, got, tc.want)
			}
		})
	}
}

// TestLayoutSamples_LinearInChunksAndEntries covers the fix for the
// quadratic cost a valid, sorted stsc table with one entry per chunk
// used to force: the old samplesInChunk rescanned entries from the
// start for every chunk, so this exact, legitimate shape (every
// chunk's own sample count changes from the last) cost time
// proportional to chunks times entries, not their sum. It is this
// test, not a malicious one, because the fix must stay fast for an
// ordinary file that happens to look like this, not only reject an
// attacker's.
func TestLayoutSamples_LinearInChunksAndEntries(t *testing.T) {
	const n = 150_000

	sizes := make([]uint32, n)
	chunkOffsets := make([]int64, n)
	entries := make([]stscEntry, n)
	for i := range n {
		entries[i] = stscEntry{firstChunk: uint32(i + 1), samplesPerChunk: 1}
	}

	start := time.Now()
	spans, ok := layoutSamples(sizes, chunkOffsets, entries)
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("layoutSamples() ok = false, want true")
	}
	if len(spans) != n {
		t.Fatalf("layoutSamples() produced %d spans, want %d", len(spans), n)
	}
	if elapsed > time.Second {
		t.Errorf("layoutSamples() took %v laying out %d chunks against %d stsc entries, want well under a second: an O(chunks*entries) scan would not finish this fast", elapsed, n, n)
	}
}

// TestAVCSamplesSafe_CancelledContextRefusesTheTrack covers
// avcSamplesSafe's own cancellation check: a context already
// cancelled before the scan starts must refuse the track outright,
// the same as any other failed check, never report it safe just
// because nothing was actually wrong with its bytes. The fixture is
// a real, well-formed single-sample track - an ordinary NAL type 5
// slice, which avcNALAllowed always passes through unjudged, so
// avcSamplesSafe would otherwise report it safe - built directly from
// box() and fixedSampleSize() rather than the external test package's
// own avc1 builders, which an internal test file cannot see.
func TestAVCSamplesSafe_CancelledContextRefusesTheTrack(t *testing.T) {
	t.Parallel()

	nal := []byte{0x05, 0x01, 0x02, 0x03} // NAL type 5 (an ordinary slice), header byte plus a few payload bytes
	sample := make([]byte, 4, 4+len(nal))
	binary.BigEndian.PutUint32(sample, uint32(len(nal)))
	sample = append(sample, nal...)

	stscBody := make([]byte, 8+12)
	binary.BigEndian.PutUint32(stscBody[4:8], 1)   // entry_count
	binary.BigEndian.PutUint32(stscBody[8:12], 1)  // first_chunk
	binary.BigEndian.PutUint32(stscBody[12:16], 1) // samples_per_chunk
	binary.BigEndian.PutUint32(stscBody[16:20], 1) // sample_description_index

	stblBody := bytes.Join([][]byte{
		box("stsz", fixedSampleSize(uint32(len(sample)), 1)),
		box("stco", make([]byte, 8+4)), // entry_count and offset patched in below
		box("stsc", stscBody),
	}, nil)
	stblBox := box("stbl", stblBody)
	data := append(bytes.Clone(stblBox), sample...)

	idx := bytes.Index(data, []byte("stco"))
	body := data[idx+4:]
	binary.BigEndian.PutUint32(body[4:8], 1)                     // entry_count
	binary.BigEndian.PutUint32(body[8:12], uint32(len(stblBox))) // the sample's own offset, right after stblBox

	c := &cursor{f: bytes.NewReader(data), size: int64(len(data)), boxes: maxBoxBudget}
	stbl := span{off: 8, len: int64(len(stblBody))}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cfg := avcDecoderConfig{lengthSize: 4}
	if c.avcSamplesSafe(ctx, stbl, 1, cfg) {
		t.Error("avcSamplesSafe() succeeded with an already-cancelled context, want refused")
	}
	if !c.avcSamplesSafe(t.Context(), stbl, 1, cfg) {
		t.Error("avcSamplesSafe() with a live context refused a well-formed sample, want this fixture itself to be valid")
	}
}
