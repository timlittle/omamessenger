package videosafe

// h264sps_internal_test.go exercises parseSPSDims and bitReader
// directly, at the bit level: the behaviour this covers - a custom
// scaling matrix refused outright, pic_order_cnt_type 1's own
// variable-length list, an Exp-Golomb prefix past maxExpGolombPrefix
// - has no shorter path through the public Validate(path) API than
// building a full MP4 fixture byte for byte, which the external test
// package's own black-box tests (isobmff_test.go) already do for the
// integration-level behaviour (a container and its SPS disagreeing,
// more than one declared SPS). This file is an internal test for
// exactly that reason: it reaches branches a realistic fixture would
// rarely hit on its own.

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// internalBitWriter builds an H.264 RBSP one bit at a time, this
// file's own minimal counterpart to bitReader, since an internal test
// file cannot see the external test package's own spsBitWriter.
type internalBitWriter struct {
	bitCount int
	buf      []byte
}

// u appends v's own low n bits, most significant bit first.
func (w *internalBitWriter) u(n int, v uint32) {
	for i := n - 1; i >= 0; i-- {
		w.bit((v >> i) & 1)
	}
}

// ue appends v Exp-Golomb-coded.
func (w *internalBitWriter) ue(v uint32) {
	codeNum := v + 1
	leadingZeros := 0
	for c := codeNum; c > 1; c >>= 1 {
		leadingZeros++
	}
	for range leadingZeros {
		w.bit(0)
	}
	w.bit(1)
	for i := leadingZeros - 1; i >= 0; i-- {
		w.bit((codeNum >> i) & 1)
	}
}

// bit appends a single bit, growing buf by a byte whenever the
// current one is full.
func (w *internalBitWriter) bit(b uint32) {
	if w.bitCount%8 == 0 {
		w.buf = append(w.buf, 0)
	}
	if b != 0 {
		w.buf[len(w.buf)-1] |= 1 << (7 - w.bitCount%8)
	}
	w.bitCount++
}

// baselineSPS builds a baseline-profile (66) SPS NAL unit for width
// and height, assumed already a multiple of 16 so no frame cropping
// is needed - everything this file's own table tests require of it.
func baselineSPS(width, height int) []byte {
	w := &internalBitWriter{}
	w.u(8, 66)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)    // seq_parameter_set_id
	w.ue(0)    // log2_max_frame_num_minus4
	w.ue(0)    // pic_order_cnt_type
	w.ue(0)    // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0)    // max_num_ref_frames
	w.u(1, 0)
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(height/16 - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 0) // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// highProfileSPS builds a profile_idc 100 (High) SPS NAL unit, with
// chroma_format_idc 1 (4:2:0) and bit depth 8, setting its own
// seq_scaling_matrix_present_flag to scalingMatrixPresent: this
// package must refuse the one case it is set, since skipping a
// custom scaling matrix's own variable-length lists correctly would
// need a full scaling_list parser this package does not implement.
func highProfileSPS(width, height int, scalingMatrixPresent uint32) []byte {
	w := &internalBitWriter{}
	w.u(8, 100)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)    // seq_parameter_set_id
	w.ue(1)    // chroma_format_idc: 4:2:0
	w.ue(0)    // bit_depth_luma_minus8
	w.ue(0)    // bit_depth_chroma_minus8
	w.u(1, 0)  // qpprime_y_zero_transform_bypass_flag
	w.u(1, scalingMatrixPresent)
	if scalingMatrixPresent != 0 {
		// A real encoder would follow this with 8 scaling-list-present
		// flags and, for each set one, a variable-length scaling_list;
		// this package refuses the SPS as soon as it sees the flag
		// itself, so none of that needs to be bit-accurate here.
		return append([]byte{0x67}, w.buf...)
	}
	w.ue(0) // log2_max_frame_num_minus4
	w.ue(0) // pic_order_cnt_type
	w.ue(0) // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0) // max_num_ref_frames
	w.u(1, 0)
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(height/16 - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 0) // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// interlacedSPS builds a baseline SPS with frame_mbs_only_flag
// cleared, exercising the field-coded height doubling and the extra
// mb_adaptive_frame_field_flag this package must still skip over.
func interlacedSPS(width, frameHeight int) []byte {
	w := &internalBitWriter{}
	w.u(8, 66)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.u(1, 0)
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(frameHeight/32 - 1)) // pic_height_in_map_units counts field lines
	w.u(1, 0)                        // frame_mbs_only_flag: cleared
	w.u(1, 0)                        // mb_adaptive_frame_field_flag
	w.u(1, 0)                        // direct_8x8_inference_flag
	w.u(1, 0)                        // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// picOrderCntType1SPS builds a baseline SPS using pic_order_cnt_type
// 1, whose own offset_for_ref_frame list this package must skip over
// correctly to reach pic_width_in_mbs_minus1 afterwards.
func picOrderCntType1SPS(width, height int, numRefFramesInCycle uint32) []byte {
	w := &internalBitWriter{}
	w.u(8, 66)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)    // seq_parameter_set_id
	w.ue(0)    // log2_max_frame_num_minus4
	w.ue(1)    // pic_order_cnt_type: 1
	w.u(1, 0)  // delta_pic_order_always_zero_flag
	w.ue(0)    // offset_for_non_ref_pic (se(v) zero)
	w.ue(0)    // offset_for_top_to_bottom_field (se(v) zero)
	w.ue(numRefFramesInCycle)
	for range numRefFramesInCycle {
		w.ue(0) // offset_for_ref_frame[i] (se(v) zero)
	}
	w.ue(0) // max_num_ref_frames
	w.u(1, 0)
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(height/16 - 1))
	w.u(1, 1)
	w.u(1, 0)
	w.u(1, 0)
	return append([]byte{0x67}, w.buf...)
}

// croppedSPSParams names every dimension croppedSPS varies: the
// declared macroblock size and the four crop offsets that follow it.
type croppedSPSParams struct {
	mbWidth, mbHeightUnits                   int
	cropLeft, cropRight, cropTop, cropBottom int
}

// croppedSPS builds a baseline-profile SPS declaring p's macroblock
// size with frame cropping enabled, p's own crop amounts given
// directly rather than derived from a target pixel size: exercises
// frameCrop's own success path, never reached by any of this file's
// other fixtures, which all leave frame_cropping_flag clear.
func croppedSPS(p croppedSPSParams) []byte {
	w := &internalBitWriter{}
	w.u(8, 66)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.u(1, 0)
	w.ue(uint32(p.mbWidth - 1))
	w.ue(uint32(p.mbHeightUnits - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 1) // frame_cropping_flag
	w.ue(uint32(p.cropLeft))
	w.ue(uint32(p.cropRight))
	w.ue(uint32(p.cropTop))
	w.ue(uint32(p.cropBottom))
	return append([]byte{0x67}, w.buf...)
}

// chromaFormatSPS builds a High-profile SPS declaring chromaFormatIDC
// and, when it is 3, separateColourPlaneFlag, with no custom scaling
// matrix and no frame cropping: the one way to reach
// parseSPSFields' own refusal of any chroma format but 4:2:0
// (chromaArrayType 1), none of which any profile-66 fixture in this
// file can reach.
func chromaFormatSPS(chromaFormatIDC, separateColourPlaneFlag uint32, mbWidth, mbHeightUnits int) []byte {
	w := &internalBitWriter{}
	w.u(8, 100) // profile_idc: High
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0, generous enough for every size this file builds
	w.ue(0)    // seq_parameter_set_id
	w.ue(chromaFormatIDC)
	if chromaFormatIDC == 3 {
		w.u(1, separateColourPlaneFlag)
	}
	w.ue(0)   // bit_depth_luma_minus8
	w.ue(0)   // bit_depth_chroma_minus8
	w.u(1, 0) // qpprime_y_zero_transform_bypass_flag
	w.u(1, 0) // seq_scaling_matrix_present_flag
	w.ue(0)   // log2_max_frame_num_minus4
	w.ue(0)   // pic_order_cnt_type
	w.ue(0)   // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0)   // max_num_ref_frames
	w.u(1, 0)
	w.ue(uint32(mbWidth - 1))
	w.ue(uint32(mbHeightUnits - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 0) // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// baselineSPSWithLimits builds a baseline-profile SPS like
// baselineSPS, but with level_idc and max_num_ref_frames given
// directly rather than fixed: the shape every one of h264BoundsOK's
// own bound tests needs, since both a declared level and a reference
// count this package must cross-check against the coded size, not
// merely read, vary independently of width and height.
func baselineSPSWithLimits(width, height int, levelIDC byte, maxNumRefFrames uint32) []byte {
	w := &internalBitWriter{}
	w.u(8, 66)
	w.u(8, 0)
	w.u(8, uint32(levelIDC))
	w.ue(0) // seq_parameter_set_id
	w.ue(0) // log2_max_frame_num_minus4
	w.ue(0) // pic_order_cnt_type
	w.ue(0) // log2_max_pic_order_cnt_lsb_minus4
	w.ue(maxNumRefFrames)
	w.u(1, 0) // gaps_in_frame_num_value_allowed_flag
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(height/16 - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 0) // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// highProfileSPSWithBitDepth builds a High-profile SPS like
// highProfileSPS, but with bit_depth_luma_minus8 and
// bit_depth_chroma_minus8 given directly rather than fixed at 0: the
// one way to reach parseSPSFields' own refusal of any bit depth but
// plain 8-bit.
func highProfileSPSWithBitDepth(width, height int, bitDepthLumaMinus8, bitDepthChromaMinus8 uint32) []byte {
	w := &internalBitWriter{}
	w.u(8, 100)
	w.u(8, 0)
	w.u(8, 50) // level_idc: 5.0
	w.ue(0)    // seq_parameter_set_id
	w.ue(1)    // chroma_format_idc: 4:2:0
	w.ue(bitDepthLumaMinus8)
	w.ue(bitDepthChromaMinus8)
	w.u(1, 0) // qpprime_y_zero_transform_bypass_flag
	w.u(1, 0) // seq_scaling_matrix_present_flag
	w.ue(0)   // log2_max_frame_num_minus4
	w.ue(0)   // pic_order_cnt_type
	w.ue(0)   // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0)   // max_num_ref_frames
	w.u(1, 0)
	w.ue(uint32(width/16 - 1))
	w.ue(uint32(height/16 - 1))
	w.u(1, 1) // frame_mbs_only_flag
	w.u(1, 0) // direct_8x8_inference_flag
	w.u(1, 0) // frame_cropping_flag
	return append([]byte{0x67}, w.buf...)
}

// TestParseSPSDims covers the fields a real SPS can vary that
// parseSPSDims must still parse correctly to reach pic_width_in_mbs
// and pic_height_in_map_units: a plain baseline SPS, a high-profile
// SPS without a custom scaling matrix, interlaced coding, and
// pic_order_cnt_type 1's own list - plus every one of those this
// package refuses rather than guesses at, including the bounds a
// differential security review found this package needed beyond the
// cropped size itself: the coded size before cropping, how much a
// crop may remove, bit depth, chroma format, and max_num_ref_frames
// both on its own and cross-checked against the SPS's own declared
// level_idc (see docs/decisions.md).
func TestParseSPSDims(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		nal        []byte
		wantWidth  int
		wantHeight int
		wantOK     bool
	}{
		"baseline profile": {
			baselineSPS(640, 480),
			640, 480, true,
		},
		"high profile without a scaling matrix": {
			highProfileSPS(1280, 720, 0),
			1280, 720, true,
		},
		"high profile with a custom scaling matrix is refused": {
			highProfileSPS(1280, 720, 1),
			0, 0, false,
		},
		"interlaced (frame_mbs_only_flag cleared)": {
			interlacedSPS(640, 480),
			640, 480, true,
		},
		"pic_order_cnt_type 1 with an offset list": {
			picOrderCntType1SPS(640, 480, 3),
			640, 480, true,
		},
		"frame cropping applied": {
			croppedSPS(croppedSPSParams{mbWidth: 10, mbHeightUnits: 10, cropLeft: 1, cropRight: 1, cropTop: 2, cropBottom: 0}),
			156, 156, true, // 160x160 macroblocks, minus 2*(crop units) per axis
		},
		"chroma_format_idc 2 (4:2:2) is refused": {
			chromaFormatSPS(2, 0, 10, 10),
			0, 0, false,
		},
		"chroma_format_idc 3 (4:4:4) is refused": {
			chromaFormatSPS(3, 0, 10, 10),
			0, 0, false,
		},
		"chroma_format_idc 3 with separate colour planes is refused": {
			chromaFormatSPS(3, 1, 10, 10),
			0, 0, false,
		},
		"bit_depth_luma_minus8 nonzero (10-bit luma) is refused": {
			highProfileSPSWithBitDepth(1280, 720, 2, 0),
			0, 0, false,
		},
		"bit_depth_chroma_minus8 nonzero (10-bit chroma) is refused": {
			highProfileSPSWithBitDepth(1280, 720, 0, 2),
			0, 0, false,
		},
		"coded size past maxDimension, even with no crop to hide it": {
			baselineSPSWithLimits(1936, 160, 60, 0),
			0, 0, false,
		},
		"a crop of exactly one macroblock column's own width is refused, on its own, within an otherwise ordinary coded size": {
			// 640x480 coded (well within maxDimension), cropping 20
			// pixels off the width alone - past maxCropSlack by
			// itself, with nothing else about this SPS a bound would
			// otherwise reject.
			croppedSPS(croppedSPSParams{mbWidth: 40, mbHeightUnits: 30, cropLeft: 10, cropRight: 0, cropTop: 0, cropBottom: 0}),
			0, 0, false,
		},
		"a crop far larger than one macroblock hides a coded size far past maxDimension": {
			croppedSPS(croppedSPSParams{mbWidth: 1000, mbHeightUnits: 1000, cropLeft: 7680, cropRight: 0, cropTop: 7000, cropBottom: 0}),
			0, 0, false, // 16000x16000 coded, cropped down to a plausible-looking 640x2000
		},
		"max_num_ref_frames at the spec's own absolute ceiling of 16 is allowed": {
			baselineSPSWithLimits(640, 480, 50, 16), // level 5.0, 1200 coded mbs: maxDpbMbs/frameSizeMbs is 92, well above 16
			640, 480, true,
		},
		"max_num_ref_frames one past the spec's own absolute ceiling of 16": {
			baselineSPSWithLimits(640, 480, 50, 17),
			0, 0, false,
		},
		"max_num_ref_frames within the absolute cap but past what the declared level allows for this coded size": {
			baselineSPSWithLimits(1920, 1920, 50, 8), // level 5.0: maxDpbMbs 110400 / 14400 mbs = 7 frames, not 8
			0, 0, false,
		},
		"a level_idc this package does not recognize": {
			baselineSPSWithLimits(640, 480, 99, 0),
			0, 0, false,
		},
		"not an SPS NAL unit": {
			[]byte{0x65, 0, 0, 0}, // nal_unit_type 5 (an IDR slice)
			0, 0, false,
		},
		"empty": {
			[]byte{},
			0, 0, false,
		},
		"truncated partway through the profile fields": {
			[]byte{0x67, 66},
			0, 0, false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			width, height, ok := parseSPSDims(tc.nal)
			if ok != tc.wantOK {
				t.Fatalf("parseSPSDims() ok = %t, want %t", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if width != tc.wantWidth || height != tc.wantHeight {
				t.Errorf("parseSPSDims() = (%d, %d), want (%d, %d)", width, height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

// TestBitReader_UeRefusesAPrefixPastTheBound covers ue's own defence
// against an Exp-Golomb prefix longer than maxExpGolombPrefix: a
// crafted SPS could otherwise encode an implausibly large leading
// dimension field this package would then have to bound-check after
// already computing it, rather than refusing it outright.
func TestBitReader_UeRefusesAPrefixPastTheBound(t *testing.T) {
	t.Parallel()

	data := make([]byte, 8) // every bit zero: an unbounded run of leading zeros
	r := &bitReader{data: data}
	if _, ok := r.ue(); ok {
		t.Error("ue() succeeded on a prefix past maxExpGolombPrefix, want it refused")
	}
}

// TestBitReader_URefusesReadingPastItsOwnData covers u's own bounds
// check once fewer bits remain than requested, so a truncated SPS can
// never read past its own buffer.
func TestBitReader_URefusesReadingPastItsOwnData(t *testing.T) {
	t.Parallel()

	r := &bitReader{data: []byte{0xFF}}
	if _, ok := r.u(9); ok {
		t.Error("u(9) succeeded reading past 8 available bits, want it refused")
	}
}

// TestParseSPSDims_TruncatedNeverSucceeds covers every one of
// parseSPSDims' own early returns at once, rather than one truncated
// byte sequence per field: for several complete, valid SPS NAL units
// exercising different structural branches, every byte-length prefix
// shorter than the whole NAL must be refused - never panic, and never
// succeed on a prefix that happens to look complete - and the whole
// NAL must still succeed.
func TestParseSPSDims_TruncatedNeverSucceeds(t *testing.T) {
	t.Parallel()

	fullNALs := map[string][]byte{
		"baseline":     baselineSPS(640, 480),
		"high profile": highProfileSPS(1280, 720, 0),
		"pic_order_cnt_type 1, several ref frames": picOrderCntType1SPS(640, 480, 5),
		"frame cropping": croppedSPS(croppedSPSParams{mbWidth: 10, mbHeightUnits: 10, cropLeft: 1, cropRight: 1, cropTop: 2, cropBottom: 0}),
	}

	for name, nal := range fullNALs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for n := 1; n < len(nal); n++ {
				if _, _, ok := parseSPSDims(nal[:n]); ok {
					t.Errorf("parseSPSDims() succeeded on %d of %d bytes, want it to need the whole NAL", n, len(nal))
				}
			}
			if _, _, ok := parseSPSDims(nal); !ok {
				t.Errorf("parseSPSDims() failed on the complete %d-byte NAL, want it to succeed", len(nal))
			}
		})
	}
}

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
	validAVCC := append([]byte{1, 66, 0, 30, 0xFF, 0x01, byte(len(sps) >> 8), byte(len(sps))}, sps...)

	t.Run("declared SPS length of zero", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, 66, 0, 30, 0xFF, 0x01, 0, 0}
		c := &cursor{f: bytes.NewReader(buf), size: int64(len(buf)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(buf))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded with a declared SPS length of zero, want it refused")
		}
	})

	t.Run("declared SPS length past maxSPSLength", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, 66, 0, 30, 0xFF, 0x01, 0xFF, 0xFF}
		c := &cursor{f: bytes.NewReader(buf), size: int64(len(buf)), boxes: maxBoxBudget}
		if _, _, ok := c.avcDecoderConfigSPSDims(span{off: 0, len: int64(len(buf))}); ok {
			t.Error("avcDecoderConfigSPSDims() succeeded with a declared SPS length past maxSPSLength, want it refused")
		}
	})

	t.Run("declared SPS length past the avcc box's own length", func(t *testing.T) {
		t.Parallel()

		buf := []byte{1, 66, 0, 30, 0xFF, 0x01, 0, 100} // declares far more than the 8 bytes actually here
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

// FuzzParseSPSDims seeds the corpus with the crafted cases an SPS
// parser must survive without panicking, hanging or reading past its
// own bounds: a valid baseline SPS, a high-profile SPS with a custom
// scaling matrix, a truncated NAL unit, one with emulation-prevention
// bytes to strip, an empty slice, and - the exact shape a differential
// security review found this package needed to bound beyond the
// cropped size alone - a huge coded size hidden behind a crop far past
// maxCropSlack, a reference-frame count past both the absolute and the
// declared level's own ceiling, a non-8-bit depth and a non-4:2:0
// chroma format. go test's mutation engine takes it from there.
func FuzzParseSPSDims(f *testing.F) {
	f.Add(baselineSPS(640, 480))
	f.Add(highProfileSPS(1280, 720, 1))
	f.Add([]byte{0x67, 0, 0, 0x03, 1, 2})
	f.Add([]byte{0x67})
	f.Add([]byte{})
	f.Add(croppedSPS(croppedSPSParams{mbWidth: 1000, mbHeightUnits: 1000, cropLeft: 7680, cropRight: 0, cropTop: 7000, cropBottom: 0}))
	f.Add(baselineSPSWithLimits(640, 480, 50, 17))
	f.Add(baselineSPSWithLimits(1920, 1920, 50, 8))
	f.Add(baselineSPSWithLimits(640, 480, 99, 0))
	f.Add(highProfileSPSWithBitDepth(1280, 720, 2, 0))
	f.Add(chromaFormatSPS(2, 0, 10, 10))

	f.Fuzz(func(t *testing.T, nal []byte) {
		parseSPSDims(nal)
	})
}
