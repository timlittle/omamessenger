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

import "testing"

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
	entriesHeader[7] = 1 // entry_count = 1
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
