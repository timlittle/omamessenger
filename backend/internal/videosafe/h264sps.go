package videosafe

// h264sps.go reads an avc1 sample entry's avcC box far enough to
// decode the single sequence parameter set (SPS) it must declare,
// and from it the coded width and height the decoder FFmpeg actually
// allocates frames at - never a sample entry's own container-declared
// width and height, which a sender fully controls independently of
// the stream's own bits (see docs/decisions.md). Every value this
// file reads is checked against the handful of bytes avcC or the SPS
// itself declare before anything is allocated or read.

import "encoding/binary"

// avcSPSDims reads trak's stbl box for its video sample entry's avcC
// configuration, and from it the single SPS the entry's own
// numOfSequenceParameterSets field must declare, returning the coded
// width and height that SPS decodes to. ok is false whenever avcC is
// missing, declares anything other than exactly one SPS - more than
// one, or zero - or that SPS cannot itself be fully parsed: an avcC
// this package cannot read all the way through is never treated as
// safe just because its container-declared dimensions looked fine.
func (c *cursor) avcSPSDims(stblOff, stblLen int64) (int, int, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 8 {
		return 0, 0, false
	}

	_, entry, _, ok := c.boxHeader(stsd.off+8, stsd.off+stsd.len)
	if !ok || entry.len < avcVisualSampleEntryFixedFields {
		return 0, 0, false
	}

	avcc, found := c.firstChild(entry.off+avcVisualSampleEntryFixedFields, entry.len-avcVisualSampleEntryFixedFields, "avcC")
	if !found {
		return 0, 0, false
	}

	return c.avcDecoderConfigSPSDims(avcc)
}

// avcDecoderConfigSPSDims reads avcc as an AVCDecoderConfigurationRecord
// (ISO/IEC 14496-15) far enough to find its first and only SPS: a
// fixed 6-byte header ending in numOfSequenceParameterSets, which must
// equal exactly one, then that one SPS's own 16-bit length and bytes,
// bounded by maxSPSLength before any of it is read.
func (c *cursor) avcDecoderConfigSPSDims(avcc span) (int, int, bool) {
	if avcc.len < 8 {
		return 0, 0, false
	}

	var hdr [6]byte
	if !c.readAt(hdr[:], avcc.off) {
		return 0, 0, false
	}
	if hdr[5]&0x1f != 1 {
		return 0, 0, false
	}

	var lengthField [2]byte
	if !c.readAt(lengthField[:], avcc.off+6) {
		return 0, 0, false
	}

	spsLen := int64(binary.BigEndian.Uint16(lengthField[:]))
	if spsLen <= 0 || spsLen > maxSPSLength || 8+spsLen > avcc.len {
		return 0, 0, false
	}

	sps := make([]byte, spsLen)
	if !c.readAt(sps, avcc.off+8) {
		return 0, 0, false
	}

	return parseSPSDims(sps)
}

// highProfileChromaFields reports whether profileIDC is one of the
// H.264 profiles whose SPS carries chroma_format_idc, bit depth and a
// scaling matrix flag before the fields this package actually needs
// - the fixed profile list Annex A of the H.264 specification gives
// for exactly this purpose, never inferred from anything in the file.
func highProfileChromaFields(profileIDC uint32) bool {
	switch profileIDC {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

// chromaSubsampling returns the subsampling factors H.264's own
// chromaArrayType implies for a value other than 0 (monochrome, which
// frameCropUnits handles separately): 4:2:0, 4:2:2 and 4:4:4
// respectively for 1, 2 and 3, the fixed table Section 6.2 of the
// specification defines.
func chromaSubsampling(chromaArrayType uint32) (subWidthC, subHeightC uint32) {
	switch chromaArrayType {
	case 2:
		return 2, 1
	case 3:
		return 1, 1
	default:
		return 2, 2
	}
}

// parseSPSDims decodes nal as a single H.264 sequence parameter set
// NAL unit and returns the coded picture width and height it
// declares, in pixels, after cropping: pic_width_in_mbs_minus1 and
// pic_height_in_map_units_minus1 scaled to pixels, adjusted for
// frame_mbs_only_flag and any frame cropping. ok is false whenever nal
// is not an SPS, runs out of bits before every field this needs, or
// declares a custom scaling matrix: skipping one correctly needs a
// full scaling_list parser this package does not implement, so an SPS
// that uses one is treated the same as any other it cannot fully
// read - not playable, never guessed at.
func parseSPSDims(nal []byte) (int, int, bool) {
	if len(nal) < 1 || nal[0]&0x1f != 7 {
		return 0, 0, false
	}

	r := &bitReader{data: removeEmulationPrevention(nal[1:])}

	profileIDC, ok := r.u(8)
	if !ok {
		return 0, 0, false
	}
	if _, ok = r.u(8); !ok { // constraint flags + reserved
		return 0, 0, false
	}
	if _, ok = r.u(8); !ok { // level_idc
		return 0, 0, false
	}
	if _, ok = r.ue(); !ok { // seq_parameter_set_id
		return 0, 0, false
	}

	chromaArrayType, ok := chromaArrayTypeOf(r, profileIDC)
	if !ok {
		return 0, 0, false
	}

	if !skipToPicSize(r) {
		return 0, 0, false
	}

	return picSizeFromSPS(r, chromaArrayType)
}

// chromaArrayTypeOf reads the chroma-related fields a high-profile
// SPS carries (chroma_format_idc, bit depths, the scaling matrix
// flag), refusing a custom scaling matrix outright, and returns the
// resulting chromaArrayType - 1 (4:2:0) for every profile that omits
// these fields entirely, the fixed default the specification assigns
// them.
func chromaArrayTypeOf(r *bitReader, profileIDC uint32) (uint32, bool) {
	if !highProfileChromaFields(profileIDC) {
		return 1, true
	}

	chromaFormatIDC, separateColourPlaneFlag, ok := readChromaFormat(r)
	if !ok {
		return 0, false
	}
	if !skipBitDepthAndScalingFlags(r) {
		return 0, false
	}

	if separateColourPlaneFlag == 1 {
		return 0, true
	}
	return chromaFormatIDC, true
}

// readChromaFormat reads chroma_format_idc and, only when it is 3,
// the separate_colour_plane_flag bit immediately after it.
func readChromaFormat(r *bitReader) (chromaFormatIDC, separateColourPlaneFlag uint32, ok bool) {
	chromaFormatIDC, ok = r.ue()
	if !ok || chromaFormatIDC != 3 {
		return chromaFormatIDC, 0, ok
	}

	separateColourPlaneFlag, ok = r.u(1)
	return chromaFormatIDC, separateColourPlaneFlag, ok
}

// skipBitDepthAndScalingFlags reads bit_depth_luma_minus8,
// bit_depth_chroma_minus8, qpprime_y_zero_transform_bypass_flag and
// seq_scaling_matrix_present_flag, refusing outright once that last
// flag is set: see parseSPSDims' own doc comment for why.
func skipBitDepthAndScalingFlags(r *bitReader) bool {
	if _, ok := r.ue(); !ok { // bit_depth_luma_minus8
		return false
	}
	if _, ok := r.ue(); !ok { // bit_depth_chroma_minus8
		return false
	}
	if _, ok := r.u(1); !ok { // qpprime_y_zero_transform_bypass_flag
		return false
	}

	scalingMatrixPresent, ok := r.u(1)
	return ok && scalingMatrixPresent == 0
}

// skipToPicSize reads every SPS field between the chroma fields
// chromaArrayTypeOf already consumed and pic_width_in_mbs_minus1,
// none of which this package needs for anything but to find its own
// place in the bitstream.
func skipToPicSize(r *bitReader) bool {
	if _, ok := r.ue(); !ok { // log2_max_frame_num_minus4
		return false
	}
	picOrderCntType, ok := r.ue()
	if !ok {
		return false
	}

	switch picOrderCntType {
	case 0:
		if _, ok = r.ue(); !ok { // log2_max_pic_order_cnt_lsb_minus4
			return false
		}
	case 1:
		if !skipPicOrderCntType1(r) {
			return false
		}
	}

	if _, ok = r.ue(); !ok { // max_num_ref_frames
		return false
	}
	if _, ok = r.u(1); !ok { // gaps_in_frame_num_value_allowed_flag
		return false
	}
	return true
}

// skipPicOrderCntType1 reads pic_order_cnt_type 1's own fields,
// including its variable-length offset_for_ref_frame list, bounded by
// how many bits actually remain so a crafted huge count cannot make
// this loop run longer than the SPS itself could possibly encode.
func skipPicOrderCntType1(r *bitReader) bool {
	if _, ok := r.u(1); !ok { // delta_pic_order_always_zero_flag
		return false
	}
	if _, ok := r.se(); !ok { // offset_for_non_ref_pic
		return false
	}
	if _, ok := r.se(); !ok { // offset_for_top_to_bottom_field
		return false
	}
	numRefFramesInCycle, ok := r.ue()
	if !ok {
		return false
	}
	for i := uint32(0); i < numRefFramesInCycle; i++ {
		if _, ok = r.se(); !ok {
			return false
		}
	}
	return true
}

// picSizeFromSPS reads pic_width_in_mbs_minus1 onward and returns the
// coded picture width and height in pixels, after frame_mbs_only_flag
// and any frame cropping are applied.
func picSizeFromSPS(r *bitReader, chromaArrayType uint32) (int, int, bool) {
	picWidthInMbsMinus1, ok := r.ue()
	if !ok {
		return 0, 0, false
	}
	picHeightInMapUnitsMinus1, ok := r.ue()
	if !ok {
		return 0, 0, false
	}
	frameMbsOnlyFlag, ok := r.u(1)
	if !ok {
		return 0, 0, false
	}
	if frameMbsOnlyFlag == 0 {
		if _, ok = r.u(1); !ok { // mb_adaptive_frame_field_flag
			return 0, 0, false
		}
	}
	if _, ok = r.u(1); !ok { // direct_8x8_inference_flag
		return 0, 0, false
	}

	width := int(picWidthInMbsMinus1+1) * 16
	height := int((2-frameMbsOnlyFlag)*(picHeightInMapUnitsMinus1+1)) * 16

	cropLeft, cropRight, cropTop, cropBottom, ok := frameCrop(r)
	if !ok {
		return 0, 0, false
	}

	cropUnitX, cropUnitY := frameCropUnits(chromaArrayType, frameMbsOnlyFlag)
	width -= int(cropUnitX * (cropLeft + cropRight))
	height -= int(cropUnitY * (cropTop + cropBottom))
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// frameCrop reads frame_cropping_flag and, when set, the four crop
// offsets that follow it, each still in the units frameCropUnits
// converts to pixels.
func frameCrop(r *bitReader) (left, right, top, bottom uint32, ok bool) {
	flag, ok := r.u(1)
	if !ok || flag == 0 {
		return 0, 0, 0, 0, ok
	}

	if left, ok = r.ue(); !ok {
		return
	}
	if right, ok = r.ue(); !ok {
		return
	}
	if top, ok = r.ue(); !ok {
		return
	}
	bottom, ok = r.ue()
	return
}

// frameCropUnits returns the pixel width and height one unit of
// frame cropping represents, Table 7-1 of the H.264 specification:
// 1x1 for monochrome (chromaArrayType 0), otherwise the chroma
// subsampling factors scaled by frame_mbs_only_flag's own field/frame
// factor on the vertical axis.
func frameCropUnits(chromaArrayType, frameMbsOnlyFlag uint32) (x, y uint32) {
	if chromaArrayType == 0 {
		return 1, 2 - frameMbsOnlyFlag
	}
	subWidthC, subHeightC := chromaSubsampling(chromaArrayType)
	return subWidthC, subHeightC * (2 - frameMbsOnlyFlag)
}

// removeEmulationPrevention strips every emulation-prevention byte
// (a 0x03 immediately after two 0x00 bytes) H.264 inserts into a NAL
// unit's own encoded bytes, returning the raw RBSP bits parseSPSDims
// reads: without this, any legitimate SPS containing two zero bytes
// in a row would misalign every bit field after them.
func removeEmulationPrevention(b []byte) []byte {
	out := make([]byte, 0, len(b))
	zeroRun := 0
	for _, v := range b {
		if zeroRun >= 2 && v == 0x03 {
			zeroRun = 0
			continue
		}
		if v == 0 {
			zeroRun++
		} else {
			zeroRun = 0
		}
		out = append(out, v)
	}
	return out
}

// bitReader reads an H.264 RBSP one bit at a time, already stripped
// of emulation prevention: a plain []byte and bit offset, never an
// io.ReaderAt, since an SPS is always small enough (maxSPSLength) to
// hold in memory as a single slice.
type bitReader struct {
	data []byte
	pos  int
}

// u reads the next n bits as an unsigned integer, most significant
// bit first, reporting false once fewer than n bits remain.
func (r *bitReader) u(n int) (uint32, bool) {
	if n > 32 || r.pos+n > len(r.data)*8 {
		return 0, false
	}

	var v uint32
	for i := 0; i < n; i++ {
		byteIdx := r.pos / 8
		bitIdx := 7 - r.pos%8
		v = v<<1 | uint32((r.data[byteIdx]>>bitIdx)&1)
		r.pos++
	}
	return v, true
}

// ue reads an Exp-Golomb-coded unsigned integer (H.264's own ue(v)):
// a run of leading zero bits, a terminating one bit, then that many
// more bits as the value's own low-order bits. A prefix longer than
// maxExpGolombPrefix is refused rather than read, the one way a
// crafted SPS could otherwise make this decode an implausibly large
// value.
func (r *bitReader) ue() (uint32, bool) {
	leadingZeros := 0
	for {
		b, ok := r.u(1)
		if !ok {
			return 0, false
		}
		if b == 1 {
			break
		}
		leadingZeros++
		if leadingZeros > maxExpGolombPrefix {
			return 0, false
		}
	}
	if leadingZeros == 0 {
		return 0, true
	}

	rest, ok := r.u(leadingZeros)
	if !ok {
		return 0, false
	}
	return (1<<leadingZeros - 1) + rest, true
}

// se reads an Exp-Golomb-coded signed integer (H.264's own se(v)),
// derived from ue the way the specification defines it: alternating
// signed values in increasing magnitude as the underlying unsigned
// code increases.
func (r *bitReader) se() (int32, bool) {
	k, ok := r.ue()
	if !ok {
		return 0, false
	}

	v := int32((k + 1) / 2)
	if k%2 == 0 {
		v = -v
	}
	return v, true
}
