package videosafe_test

// avcsamples_test.go checks Validate against avc1 fixtures whose
// samples carry real, length-prefixed NAL units (see helpers_test.go's
// mp4AVCVideoWithSamples), covering the exact gap a differential
// security review found: avcC's own SPS agreeing with the container
// is not enough on its own, since a sample can still carry a second,
// untrusted parameter set FFmpeg would still honor.

import (
	"encoding/binary"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4InBandParameterSets covers every shape
// avcsamples.go's own in-band NAL scan must judge: ordinary slice
// data, an in-band SPS identical to avcC's own (allowed), a distinct
// in-band SPS (the exploit scenario the review found - refused), an
// SPS extension and a subset SPS (always refused), a PPS naming the
// known SPS id (allowed) or an unknown one (refused), and a track
// whose sample layout cannot be determined at all.
func TestValidate_MP4InBandParameterSets(t *testing.T) {
	t.Parallel()

	sameSPS := buildAVCSPS(1280, 720)
	otherSPS := buildAVCSPS(16000, 16000)

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"ordinary slice data only": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCSliceNAL(5))},
			}),
			true,
		},
		"in-band SPS identical to avcC's own": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(sameSPS, buildAVCSliceNAL(5))},
			}),
			true,
		},
		"in-band SPS distinct from avcC's own": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(otherSPS, buildAVCSliceNAL(5))},
			}),
			false,
		},
		"in-band SPS extension": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCSliceNAL(13), buildAVCSliceNAL(5))},
			}),
			false,
		},
		"in-band subset SPS": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCSliceNAL(15), buildAVCSliceNAL(5))},
			}),
			false,
		},
		"in-band PPS naming the known SPS id": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCPPS(0), buildAVCSliceNAL(5))},
			}),
			true,
		},
		"in-band PPS naming an unknown SPS id": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCPPS(1), buildAVCSliceNAL(5))},
			}),
			false,
		},
		"in-band parameter set across several samples, one later sample bad": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{
					avcSample(buildAVCSliceNAL(5)),
					avcSample(buildAVCSliceNAL(1)),
					avcSample(otherSPS, buildAVCSliceNAL(5)),
				},
			}),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "inband.mp4", tc.data)
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t", got, tc.want)
			}
		})
	}
}

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

// TestValidate_MP4InBandNALFraming covers the sample-framing edge
// cases avcNALAllowed and scanAVCSample must judge directly: a sample
// too short to even hold one length prefix, a NAL length that claims
// more bytes than remain in the sample, a zero-length NAL, and a PPS
// truncated before its own seq_parameter_set_id can be read.
func TestValidate_MP4InBandNALFraming(t *testing.T) {
	t.Parallel()

	truncatedPPS := []byte{0x68, 0x80} // pic_parameter_set_id only; nothing left for seq_parameter_set_id

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"sample shorter than one length prefix": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{{0x00, 0x01}},
			}),
			false,
		},
		"NAL length exceeds the remaining sample bytes": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{{0x00, 0x00, 0x00, 0x64, 0x05}}, // declares length 100, one byte follows
			}),
			false,
		},
		"zero-length NAL": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{{0x00, 0x00, 0x00, 0x00}},
			}),
			false,
		},
		"PPS truncated before its own seq_parameter_set_id": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(truncatedPPS, buildAVCSliceNAL(5))},
			}),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "framing.mp4", tc.data)
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestValidate_MP4InBandSampleLayoutVariants covers every other shape
// a real sample layout can take that the single-chunk, variable-size
// fixtures above never reach: a fixed sample size shared by every
// sample (stsz's own sample_size field, rather than a per-sample
// table), 64-bit chunk offsets (co64), several chunks (exercising
// stsc's own multi-entry table and the extra, unused trailing chunk a
// real file can still declare), a track whose stsz is replaced by
// stz2 (unsupported, so the whole track is refused), and one with no
// stsc at all.
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

// FuzzValidateMP4InBandSamples seeds the corpus with every shape
// avcsamples.go's own sample-layout and NAL-unit walkers must survive
// without panicking, hanging or reading past their own bounds: a file
// with ordinary sample data, one with an in-band SPS (identical and
// distinct), one with an in-band PPS, one with a truncated sample,
// and one truncated partway through the sample-layout boxes
// themselves. go test's mutation engine takes it from there.
func FuzzValidateMP4InBandSamples(f *testing.F) {
	sameSPS := buildAVCSPS(1280, 720)
	otherSPS := buildAVCSPS(16000, 16000)

	ordinary := mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(buildAVCSliceNAL(5))},
	})
	f.Add(ordinary)
	f.Add(mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(sameSPS, buildAVCSliceNAL(5))},
	}))
	f.Add(mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(otherSPS, buildAVCSliceNAL(5))},
	}))
	f.Add(mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(buildAVCPPS(0), buildAVCSliceNAL(5))},
	}))
	f.Add(ordinary[:len(ordinary)-2]) // truncated partway through the last sample

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz-inband.mp4", data)
		if _, err := videosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
