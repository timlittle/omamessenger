package videosafe_test

// avcsampleslayout_test.go checks Validate against avc1 fixtures that
// vary a track's own sample *layout* (stsz/stz2, stco/co64, stsc)
// rather than what NAL-level content a sample itself carries - split
// out of avcsamples_test.go, whose own in-band NAL-judging tests this
// package's own file-length limit would otherwise have to share room
// with, once this package's own layout walker (avcsamples.go) grew
// enough edge cases of its own to need them.

import (
	"encoding/binary"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4InBandSampleLayoutUndetermined covers a video track
// that declares samples but whose byte layout cannot be determined at
// all - no stco/co64, or a stz2 table instead of stsz - which this
// package refuses outright rather than skip the scan.
func TestValidate_MP4InBandSampleLayoutUndetermined(t *testing.T) {
	t.Parallel()

	withoutChunkOffsets := func() []byte {
		entryBody := make([]byte, 78)
		binary.BigEndian.PutUint16(entryBody[24:26], 1280)
		binary.BigEndian.PutUint16(entryBody[26:28], 720)
		entryBody = append(entryBody, mp4Box("avcC", buildAVCDecoderConfig(1280, 720))...)
		entry := mp4Box("avc1", entryBody)

		stsdBody := make([]byte, 8, 8+len(entry))
		binary.BigEndian.PutUint32(stsdBody[4:8], 1) // entry_count
		stsdBody = append(stsdBody, entry...)

		samples := [][]byte{avcSample(buildAVCSliceNAL(5))}
		stbl := mp4Box("stbl", concatBytes(
			mp4Box("stsd", stsdBody),
			mp4Box("stsz", mp4SampleSizeTable(samples)),
			mp4Box("stsc", mp4SampleToChunk(1)),
			// no stco/co64 at all: the sample's own bytes have nowhere
			// this package can locate them from.
		))
		minf := mp4Box("minf", stbl)
		mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
		hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
		mdia := mp4Box("mdia", concatBytes(mdhd, hdlr, minf))
		trak := mp4Box("trak", mdia)
		mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
		moov := mp4Box("moov", concatBytes(mvhd, trak))
		ftyp := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
		return concatBytes(ftyp, moov)
	}()

	path := writeFixture(t, "nolayout.mp4", withoutChunkOffsets)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a video track with no stco/co64 to locate its own samples, want false")
	}
}

// TestValidate_MP4InBandSampleLayoutVariants covers every other shape
// a real sample layout can take that the single-chunk, variable-size
// fixtures elsewhere in this package never reach: a fixed sample size
// shared by every sample (stsz's own sample_size field, rather than a
// per-sample table), 64-bit chunk offsets (co64), several chunks
// (exercising stsc's own multi-entry table and the extra, unused
// trailing chunk a real file can still declare), a track whose stsz
// is replaced by stz2 (unsupported, so the whole track is refused),
// and one with no stsc at all.
func TestValidate_MP4InBandSampleLayoutVariants(t *testing.T) {
	t.Parallel()

	t.Run("fixed sample size", func(t *testing.T) {
		t.Parallel()

		sample := avcSample(buildAVCSliceNAL(5))
		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: fixedStszBody(uint32(len(sample)), 1),
			stco: stcoBody(0),
			stsc: stscBody([2]uint32{1, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, true, "a track whose stsz declares a fixed sample size")
	})

	t.Run("64-bit chunk offsets", func(t *testing.T) {
		t.Parallel()

		sample := avcSample(buildAVCSliceNAL(5))
		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(sample))),
			co64: co64Body(0),
			stsc: stscBody([2]uint32{1, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchCo64Offset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, true, "a track whose chunk offsets use co64")
	})

	t.Run("three samples across two chunks, plus an unused trailing chunk", func(t *testing.T) {
		t.Parallel()

		s1 := avcSample(buildAVCSliceNAL(5))
		s2 := avcSample(buildAVCSliceNAL(1))
		s3 := avcSample(buildAVCSliceNAL(1))
		mdat := concatBytes(s1, s2, s3)

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(s1)), uint32(len(s2)), uint32(len(s3))),
			stco: stcoBody(0, 0, 0), // three placeholders, patched below
			stsc: stscBody([2]uint32{1, 2}, [2]uint32{2, 1}),
			mdat: mdat,
		}
		data := mp4AVCVideoCustomLayout(p)
		base := int64(len(data) - len(mdat))
		off1 := base
		off2 := base + int64(len(s1)+len(s2))
		patchStcoOffsets(data, []int64{off1, off2, off2}) // the third chunk is never used

		assertPlayable(t, data, true, "three ordinary samples laid out across two real chunks and one unused one")
	})

	t.Run("stz2 instead of stsz, with real samples declared", func(t *testing.T) {
		t.Parallel()

		sample := avcSample(buildAVCSliceNAL(5))
		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stz2: mp4SampleSize(1),
			stco: stcoBody(0),
			stsc: stscBody([2]uint32{1, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, false, "a video track with samples declared through stz2 rather than stsz")
	})

	t.Run("stco present, no stsc at all", func(t *testing.T) {
		t.Parallel()

		sample := avcSample(buildAVCSliceNAL(5))
		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(sample))),
			stco: stcoBody(0),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, false, "a video track with stco but no stsc at all")
	})
}

// TestValidate_MP4InBandSampleBoundsExceeded covers the two distinct
// ways a sample's own declared position can exceed the file's real
// size: one sample, on its own, placed past the end of the file, and
// several samples whose lengths alone sum past it even though each
// individually still looks small.
func TestValidate_MP4InBandSampleBoundsExceeded(t *testing.T) {
	t.Parallel()

	t.Run("one sample's own offset and length exceed the file", func(t *testing.T) {
		t.Parallel()

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(100),
			stco: stcoBody(1_000_000), // far past any real file this small
			stsc: stscBody([2]uint32{1, 1}),
		}
		data := mp4AVCVideoCustomLayout(p)

		assertPlayable(t, data, false, "a sample whose own offset puts it past the end of the file")
	})

	t.Run("sample lengths sum past the file even though each is individually small", func(t *testing.T) {
		t.Parallel()

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(0, 1_000_000), // the first sample is free (zero bytes); the second is not
			stco: stcoBody(0),
			stsc: stscBody([2]uint32{1, 2}),
		}
		data := mp4AVCVideoCustomLayout(p)

		assertPlayable(t, data, false, "sample lengths that sum past the file's own size")
	})
}

// TestValidate_MP4InBandMalformedStscRows covers every way a crafted
// stsc table can depart from ISO/IEC 14496-12's own shape - out of
// order, a duplicated first_chunk, one that does not start at chunk
// 1, a zero samples_per_chunk row, or a sample_description_index
// other than 1 - each of which the layout walker's old, unbounded
// rescan let through (and a zero row let an attacker defeat its own
// early-break optimization with; see avcsamples.go's own doc
// comment). Every one must be refused outright, the same "cannot
// read it, cannot trust it" rule every other malformed table in this
// package already gets.
func TestValidate_MP4InBandMalformedStscRows(t *testing.T) {
	t.Parallel()

	sample := avcSample(buildAVCSliceNAL(5))

	cases := map[string][2]uint32{
		"zero samples_per_chunk": {1, 0},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := avcCustomLayoutParams{
				width: 1280, height: 720,
				stsz: stszBody(uint32(len(sample))),
				stco: stcoBody(0),
				stsc: stscBody(entry),
				mdat: sample,
			}
			data := mp4AVCVideoCustomLayout(p)
			patchStcoOffset(data, int64(len(data)-len(p.mdat)))

			assertPlayable(t, data, false, name)
		})
	}

	t.Run("does not start at chunk 1", func(t *testing.T) {
		t.Parallel()

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(sample))),
			stco: stcoBody(0),
			stsc: stscBody([2]uint32{2, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, false, "stsc not starting at chunk 1")
	})

	t.Run("out of order", func(t *testing.T) {
		t.Parallel()

		s1 := avcSample(buildAVCSliceNAL(5))
		s2 := avcSample(buildAVCSliceNAL(1))
		mdat := concatBytes(s1, s2)

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(s1)), uint32(len(s2))),
			stco: stcoBody(0, 0),
			stsc: stscBody([2]uint32{2, 1}, [2]uint32{1, 1}), // chunk 2 declared before chunk 1
			mdat: mdat,
		}
		data := mp4AVCVideoCustomLayout(p)
		base := int64(len(data) - len(mdat))
		patchStcoOffsets(data, []int64{base, base + int64(len(s1))})

		assertPlayable(t, data, false, "stsc rows out of order")
	})

	t.Run("duplicate first_chunk", func(t *testing.T) {
		t.Parallel()

		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(sample))),
			stco: stcoBody(0),
			stsc: stscBody([2]uint32{1, 1}, [2]uint32{1, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, false, "duplicate first_chunk rows")
	})

	t.Run("sample_description_index other than 1", func(t *testing.T) {
		t.Parallel()

		// The one sample entry sampleEntryCodec ever reads is always
		// index 1; a chunk's samples claiming any other index name a
		// sample description this package never validated at all.
		p := avcCustomLayoutParams{
			width: 1280, height: 720,
			stsz: stszBody(uint32(len(sample))),
			stco: stcoBody(0),
			stsc: stscBodyWithIndex(2, [2]uint32{1, 1}),
			mdat: sample,
		}
		data := mp4AVCVideoCustomLayout(p)
		patchStcoOffset(data, int64(len(data)-len(p.mdat)))

		assertPlayable(t, data, false, "stsc entry naming sample_description_index 2")
	})
}

// assertPlayable runs Validate on data and reports a test failure
// naming what scenario was being checked when the result did not
// match want.
func assertPlayable(t *testing.T, data []byte, want bool, scenario string) {
	t.Helper()

	path := writeFixture(t, "layout.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Validate() = %t, want %t, for %s", got, want, scenario)
	}
}
