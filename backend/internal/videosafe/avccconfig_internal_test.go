package videosafe

// avccconfig_internal_test.go exercises avccconfig.go's own
// avcC-box-level reading directly - avcSPSDims' structural search,
// and readAVCDecoderConfig's own bounds and failure paths - the same
// reason h264sps_internal_test.go is an internal test: reaching these
// branches through a full MP4 fixture would need either an unrealistic
// read failure mid-box, or (for the high-profile chroma extension) a
// container-level high-profile SPS builder this package has no other
// use for. isobmff_test.go covers the integration-level behaviour a
// security review actually found (a container and its avcC
// disagreeing) black-box, through Validate itself.

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestAvcSPSDims_StructuralEdgeCases covers avcSPSDims' own search
// for the sample entry and its avcC child, each checked directly
// since this is the one package function trackInfo's own call order
// never reaches with a short-enough stsd or entry to fail here first
// - sampleEntryCodec already requires a usable stsd before avcSPSDims
// is ever called.
func TestAvcSPSDims_StructuralEdgeCases(t *testing.T) {
	t.Parallel()

	stsdWith := func(entry []byte) []byte {
		buf := make([]byte, 8, 8+len(entry))
		binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
		buf = append(buf, entry...)
		return box("stsd", buf)
	}

	cases := map[string][]byte{
		"no stsd at all":                                        box("vmhd", []byte{}),
		"stsd too short to hold a header":                       box("stsd", make([]byte, 4)),
		"entry shorter than the fixed VisualSampleEntry fields": stsdWith(box("avc1", make([]byte, 10))),
		"entry with no avcC child":                              stsdWith(box("avc1", make([]byte, 78))),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: bytes.NewReader(body), size: int64(len(body)), boxes: maxBoxBudget}
			if _, _, ok := c.avcSPSDims(0, int64(len(body))); ok {
				t.Errorf("avcSPSDims() succeeded for %q, want it refused", name)
			}
		})
	}
}

// TestAvcDecoderConfigSPSDims_EdgeCases covers avcDecoderConfigSPSDims'
// own bounds checks on the SPS length field, and every one of its
// reads failing partway through - something no well-formed file can
// reach, the same reason readfailure_internal_test.go exists for the
// ISO BMFF and EBML walkers.
func TestAvcDecoderConfigSPSDims_EdgeCases(t *testing.T) {
	t.Parallel()

	sps := baselineSPS(640, 480)
	validAVCC := append([]byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01, byte(len(sps) >> 8), byte(len(sps))}, sps...)

	t.Run("declared SPS length of zero", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01, 0, 0}
		c := &cursor{f: bytes.NewReader(buf), size: int64(len(buf)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(buf))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded with a declared SPS length of zero, want it refused")
		}
	})

	t.Run("declared SPS length past maxSPSLength", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01, 0xFF, 0xFF}
		c := &cursor{f: bytes.NewReader(buf), size: int64(len(buf)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(buf))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded with a declared SPS length past maxSPSLength, want it refused")
		}
	})

	t.Run("declared SPS length past the avcc box's own length", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01, 0, 100} // declares far more than the 8 bytes actually here
		c := &cursor{f: bytes.NewReader(buf), size: int64(len(buf)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(buf))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded with a declared SPS length past the avcc box, want it refused")
		}
	})

	t.Run("fixed header read fails", func(t *testing.T) {
		t.Parallel()

		c := &cursor{f: &endFailingReaderAt{data: validAVCC, failPast: 4}, size: int64(len(validAVCC)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(validAVCC))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded once its own header read failed, want it refused")
		}
	})

	t.Run("SPS length field read fails", func(t *testing.T) {
		t.Parallel()

		c := &cursor{f: &endFailingReaderAt{data: validAVCC, failPast: 7}, size: int64(len(validAVCC)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(validAVCC))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded once its own SPS-length read failed, want it refused")
		}
	})

	t.Run("SPS bytes read fails", func(t *testing.T) {
		t.Parallel()

		c := &cursor{f: &endFailingReaderAt{data: validAVCC, failPast: int64(len(validAVCC) - 1)}, size: int64(len(validAVCC)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(validAVCC))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded once its own SPS-bytes read failed, want it refused")
		}
	})
}

// TestReadAVCDecoderConfig_TrailingData covers avcCTrailingDataAllowed
// and avcCProfileBytesMatchSPS directly, the bounds a security review
// found readAVCDecoderConfig never checked: avcC's own
// numOfPictureParameterSets list, parsed and skipped but never itself
// judged, the one high-profile chroma/bit-depth extension this
// package accepts when it is consistent with the SPS it already
// bound, and every way trailing data is refused instead - a PPS list
// that runs past avcC's own declared length, a profile that gets no
// extension at all still carrying trailing bytes, and an extension
// whose own fields disagree with the SPS. An internal test for the
// same reason TestAvcDecoderConfigSPSDims_EdgeCases is: reaching the
// high-profile extension's own byte layout through a full MP4 fixture
// would need a container-level high-profile SPS builder this package
// has no other use for.
func TestReadAVCDecoderConfig_TrailingData(t *testing.T) {
	t.Parallel()

	buildAVCC := func(sps []byte, numPPS byte, ppsEntries [][]byte, extra []byte) []byte {
		buf := []byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01}
		buf = append(buf, byte(len(sps)>>8), byte(len(sps)))
		buf = append(buf, sps...)
		buf = append(buf, numPPS)
		for _, pps := range ppsEntries {
			buf = append(buf, byte(len(pps)>>8), byte(len(pps)))
			buf = append(buf, pps...)
		}
		return append(buf, extra...)
	}

	baseline := baselineSPS(640, 480)
	highProfile := highProfileSPS(1280, 720, 0)
	pps := []byte{0x68, 0x80}

	buildAVCCNoTrailer := func(sps []byte) []byte {
		buf := []byte{1, sps[1], sps[2], sps[3], 0xFF, 0x01}
		buf = append(buf, byte(len(sps)>>8), byte(len(sps)))
		return append(buf, sps...)
	}

	cases := map[string]struct {
		avcc []byte
		ok   bool
	}{
		"baseline, no numOfPictureParameterSets byte at all": {
			// ISO/IEC 14496-15 makes the byte mandatory, but a real-world
			// fixture elsewhere in this codebase omits it; since avcC ends
			// exactly at the SPS here, there is no room left to hide
			// anything either way, so this must still be accepted.
			buildAVCCNoTrailer(baseline),
			true,
		},
		"baseline, no PPS, nothing trailing": {
			buildAVCC(baseline, 0, nil, nil),
			true,
		},
		"baseline, one PPS, nothing trailing": {
			buildAVCC(baseline, 1, [][]byte{pps}, nil),
			true,
		},
		"baseline, PPS list runs past avcC's own length": {
			append(buildAVCC(baseline, 1, nil, nil), 0, 100), // declares a PPS longer than anything left
			false,
		},
		"baseline, unparsed trailing bytes": {
			buildAVCC(baseline, 0, nil, []byte{0x11, 0x22, 0x33, 0x44}),
			false,
		},
		"high profile, correct chroma extension": {
			buildAVCC(highProfile, 0, nil, []byte{0xFD, 0xF8, 0xF8, 0x00}),
			true,
		},
		"high profile, extension declares the wrong chroma format": {
			buildAVCC(highProfile, 0, nil, []byte{0xFE, 0xF8, 0xF8, 0x00}),
			false,
		},
		"high profile, extension declares a non-zero SPS extension count": {
			buildAVCC(highProfile, 0, nil, []byte{0xFD, 0xF8, 0xF8, 0x01}),
			false,
		},
		"high profile, no extension but trailing bytes anyway": {
			buildAVCC(highProfile, 0, nil, []byte{0x00}),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: bytes.NewReader(tc.avcc), size: int64(len(tc.avcc)), boxes: maxBoxBudget}
			_, ok := c.readAVCDecoderConfig(span{off: 0, len: int64(len(tc.avcc))})
			if ok != tc.ok {
				t.Errorf("readAVCDecoderConfig() ok = %t for %q, want %t", ok, name, tc.ok)
			}
		})
	}
}
