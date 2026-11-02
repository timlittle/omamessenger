package videosafe_test

// avcsamples_test.go checks Validate against avc1 fixtures whose
// samples carry real, length-prefixed NAL units (see helpers_test.go's
// mp4AVCVideoWithSamples), covering the exact gap a differential
// security review found: avcC's own SPS agreeing with the container
// is not enough on its own, since a sample can still carry a second,
// untrusted parameter set FFmpeg would still honor.

import (
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
		"in-band NAL type 0 (unspecified)": {
			// A security review found this the exact type a sample's
			// leading bytes read as under avcC's own "sample shaped like a
			// replacement parameter-set record" heuristic (see
			// TestValidate_MP4InBandAVCCShapedSample), yet this package
			// left it passing through unjudged.
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCSliceNAL(0), buildAVCSliceNAL(5))},
			}),
			false,
		},
		"in-band NAL type 24 (reserved)": {
			mp4AVCVideoWithSamples(avcVideoSamplesParams{
				width: 1280, height: 720,
				samples: [][]byte{avcSample(buildAVCSliceNAL(24), buildAVCSliceNAL(5))},
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

// TestValidate_MP4InBandAVCCShapedSample, the exact mechanism a
// security review found - a sample shaped like a replacement
// AVCDecoderConfigurationRecord - lives in its own file,
// avccshapedsample_test.go: the shape it builds is as much about
// avcC's own header bytes as the sample itself.

// TestValidate_MP4InBandSampleLayoutUndetermined,
// TestValidate_MP4InBandSampleLayoutVariants,
// TestValidate_MP4InBandSampleBoundsExceeded and
// TestValidate_MP4InBandMalformedStscRows - every case that varies a
// track's own sample *layout* rather than a sample's NAL content -
// live in avcsampleslayout_test.go, along with the assertPlayable
// helper they share.

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

// TestValidate_MP4InBandSampleLayoutVariants,
// TestValidate_MP4InBandSampleBoundsExceeded and
// TestValidate_MP4InBandMalformedStscRows, plus the assertPlayable
// helper they share, live in avcsampleslayout_test.go.

// FuzzValidateMP4InBandSamples seeds the corpus with every shape
// avcsamples.go's own sample-layout and NAL-unit walkers must survive
// without panicking, hanging or reading past their own bounds: a file
// with ordinary sample data, one with an in-band SPS (identical and
// distinct), one with an in-band PPS, a NAL type 0 and a reserved
// type, one with a sample shaped like a replacement
// AVCDecoderConfigurationRecord (the exact mechanism a security
// review found - see TestValidate_MP4InBandAVCCShapedSample), one
// with a truncated sample, and one truncated partway through the
// sample-layout boxes themselves. go test's mutation engine takes it
// from there.
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
	f.Add(mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(buildAVCSliceNAL(0), buildAVCSliceNAL(5))},
	}))
	f.Add(mp4AVCVideoWithSamples(avcVideoSamplesParams{
		width: 1280, height: 720,
		samples: [][]byte{avcSample(buildAVCSliceNAL(24), buildAVCSliceNAL(5))},
	}))
	f.Add(func() []byte {
		avccShapedSample := append([]byte{
			1, 0, 0, 0x32, 0xFF, 0xE1,
			0, 4, 0x67, 0x01, 0x02, 0x03,
			1,
			0, 2, 0x68, 0x80,
		}, make([]byte, 258-17)...)
		return mp4AVCVideoFixtureWithAVCC(1280, 720, buildAVCDecoderConfigWithLengthSize(1280, 720, 1), [][]byte{avccShapedSample})
	}())
	f.Add(ordinary[:len(ordinary)-2]) // truncated partway through the last sample

	malformedStscSample := avcSample(buildAVCSliceNAL(5))
	malformedStscParams := avcCustomLayoutParams{
		width: 1280, height: 720,
		stsz: stszBody(uint32(len(malformedStscSample))),
		stco: stcoBody(0),
		stsc: stscBody([2]uint32{1, 0}), // zero samples_per_chunk: refused outright, not just skipped
		mdat: malformedStscSample,
	}
	malformedStscData := mp4AVCVideoCustomLayout(malformedStscParams)
	patchStcoOffset(malformedStscData, int64(len(malformedStscData)-len(malformedStscParams.mdat)))
	f.Add(malformedStscData)

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz-inband.mp4", data)
		if _, err := videosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
