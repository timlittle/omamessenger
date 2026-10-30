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
