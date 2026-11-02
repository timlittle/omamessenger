package videosafe

// h264sps.go decodes an H.264 sequence parameter set (SPS) NAL unit's
// own bits: the coded width and height the decoder FFmpeg actually
// allocates frames at - never a sample entry's own container-declared
// width and height, which a sender fully controls independently of
// the stream's own bits (see docs/decisions.md) - plus every other
// field this package needs to bound what FFmpeg's decoder will do
// with it (profile, level, bit depth, chroma format, reference-frame
// count). avccconfig.go reads the avcC box this SPS comes from and
// calls into this file to parse it; this file has no idea what box or
// sample it was found in. avcDecoderConfig, the struct that box
// reading fills in from this file's own results, is declared in
// avccconfig.go alongside the rest of that box-level code.
// highProfileChromaFields - the one other function this file shares
// with avccconfig.go, since both need the same Annex A profile list
// - is declared there too.

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
	width, height, _, ok := parseSPSFields(nal)
	return width, height, ok
}

// parseSPSFields decodes nal exactly as parseSPSDims does, and
// additionally returns the SPS's own seq_parameter_set_id: the id
// avcC's one trusted SPS declares, which avcsamples.go's own in-band
// scan compares every picture parameter set found inside a sample
// against (see avcNALAllowed). ok is also false whenever the SPS
// declares anything outside what this package has bound FFmpeg's own
// decoder to: a coded size past maxDimension even once cropping is
// undone, a crop larger than one macroblock's own slack, a bit depth
// other than 8, a chroma format other than 4:2:0, or a reference-frame
// count or coded-frame memory cost past what either a fixed cap or
// the SPS's own declared level_idc allows (see docs/decisions.md).
func parseSPSFields(nal []byte) (width, height int, spsID uint32, ok bool) {
	if len(nal) < 1 || nal[0]&0x1f != 7 {
		return 0, 0, 0, false
	}

	r := &bitReader{data: removeEmulationPrevention(nal[1:])}

	profileIDC, levelIDC, spsID, ok := readSPSHeader(r)
	if !ok {
		return 0, 0, 0, false
	}

	width, height, ok = parseSPSBody(r, profileIDC, levelIDC)
	return width, height, spsID, ok
}

// readSPSHeader reads an SPS's own leading fields, before any
// profile-dependent branching: profile_idc, the constraint-flags and
// reserved byte (never used for anything), level_idc, and
// seq_parameter_set_id.
func readSPSHeader(r *bitReader) (profileIDC, levelIDC, spsID uint32, ok bool) {
	profileIDC, ok = r.u(8)
	if !ok {
		return 0, 0, 0, false
	}
	if _, ok = r.u(8); !ok { // constraint flags + reserved
		return 0, 0, 0, false
	}
	levelIDC, ok = r.u(8)
	if !ok {
		return 0, 0, 0, false
	}
	spsID, ok = r.ue() // seq_parameter_set_id
	return profileIDC, levelIDC, spsID, ok
}

// parseSPSBody reads everything in an SPS after its own header
// fields - chroma format and bit depth, every field skipToPicSize
// finds, and finally the coded and cropped picture size - and applies
// every bound h264BoundsOK checks against profileIDC's chroma fields
// and levelIDC.
func parseSPSBody(r *bitReader, profileIDC, levelIDC uint32) (width, height int, ok bool) {
	chromaArrayType, eightBit, ok := chromaArrayTypeOf(r, profileIDC)
	if !ok || chromaArrayType != 1 || !eightBit {
		return 0, 0, false
	}

	maxNumRefFrames, ok := skipToPicSize(r)
	if !ok {
		return 0, 0, false
	}

	codedWidth, codedHeight, width, height, ok := picSizeFromSPS(r, chromaArrayType)
	if !ok || !h264BoundsOK(levelIDC, codedWidth, codedHeight, maxNumRefFrames) {
		return 0, 0, false
	}
	return width, height, true
}

// h264BoundsOK reports whether a coded frame size of codedWidth by
// codedHeight pixels, with maxNumRefFrames declared reference frames,
// is one FFmpeg's own H.264 decoder can be trusted to allocate for:
// both axes within maxDimension, the reference count within
// maxH264RefFrames, the combined coded-pixel/reference-frame cost
// within maxCodedFrameBudget, and - a cross-check against what the
// SPS's own declared level_idc (Annex A Table A-1) additionally
// allows for a frame this size - levelIDC must be one this package
// recognizes, the coded frame size must fit its own maxFS, and the
// reference count must fit the reference-frame buffer count its own
// maxDpbMbs implies for a frame this size.
func h264BoundsOK(levelIDC uint32, codedWidth, codedHeight int, maxNumRefFrames uint32) bool {
	if !dimensionsWithinCap(codedWidth, codedHeight) || maxNumRefFrames > maxH264RefFrames {
		return false
	}
	if int64(codedWidth)*int64(codedHeight)*(int64(maxNumRefFrames)+1) > maxCodedFrameBudget {
		return false
	}

	limits, known := h264Levels[levelIDC]
	if !known {
		return false
	}

	frameSizeMbs := int64(codedWidth/16) * int64(codedHeight/16)
	if frameSizeMbs <= 0 || frameSizeMbs > limits.maxFS {
		return false
	}

	maxDpbFrames := limits.maxDpbMbs / frameSizeMbs
	if maxDpbFrames > maxH264RefFrames {
		maxDpbFrames = maxH264RefFrames
	}
	return int64(maxNumRefFrames) <= maxDpbFrames
}

// chromaArrayTypeOf reads the chroma-related fields a high-profile
// SPS carries (chroma_format_idc, bit depths, the scaling matrix
// flag), refusing a custom scaling matrix outright, and returns the
// resulting chromaArrayType and whether both bit depths read as plain
// 8-bit (bit_depth_luma_minus8 and bit_depth_chroma_minus8 both 0).
// A profile that omits these fields entirely gets the fixed defaults
// the specification assigns them: chromaArrayType 1 (4:2:0), and
// 8-bit, since those profiles have no other bit depth to declare.
func chromaArrayTypeOf(r *bitReader, profileIDC uint32) (chromaArrayType uint32, eightBit, ok bool) {
	if !highProfileChromaFields(profileIDC) {
		return 1, true, true
	}

	chromaFormatIDC, separateColourPlaneFlag, ok := readChromaFormat(r)
	if !ok {
		return 0, false, false
	}
	eightBit, ok = readBitDepthAndScalingFlags(r)
	if !ok {
		return 0, false, false
	}

	if separateColourPlaneFlag == 1 {
		return 0, eightBit, true
	}
	return chromaFormatIDC, eightBit, true
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

// readBitDepthAndScalingFlags reads bit_depth_luma_minus8,
// bit_depth_chroma_minus8, qpprime_y_zero_transform_bypass_flag and
// seq_scaling_matrix_present_flag, refusing outright once that last
// flag is set: see parseSPSDims' own doc comment for why. eightBit
// reports whether both bit depths read as 0 (plain 8-bit): this
// package never bounds the far larger per-pixel decode buffer a
// higher bit depth would need, so anything else is left to the
// caller to refuse.
func readBitDepthAndScalingFlags(r *bitReader) (eightBit, ok bool) {
	bitDepthLumaMinus8, ok := r.ue()
	if !ok {
		return false, false
	}
	bitDepthChromaMinus8, ok := r.ue()
	if !ok {
		return false, false
	}
	if _, ok = r.u(1); !ok { // qpprime_y_zero_transform_bypass_flag
		return false, false
	}

	scalingMatrixPresent, ok := r.u(1)
	if !ok || scalingMatrixPresent != 0 {
		return false, false
	}
	return bitDepthLumaMinus8 == 0 && bitDepthChromaMinus8 == 0, true
}

// skipToPicSize reads every SPS field between the chroma fields
// chromaArrayTypeOf already consumed and pic_width_in_mbs_minus1,
// returning maxNumRefFrames - the one field among them h264BoundsOK
// needs - alongside ok; every other field read here only finds this
// package's own place in the bitstream.
func skipToPicSize(r *bitReader) (maxNumRefFrames uint32, ok bool) {
	if _, ok = r.ue(); !ok { // log2_max_frame_num_minus4
		return 0, false
	}
	picOrderCntType, ok := r.ue()
	if !ok {
		return 0, false
	}

	switch picOrderCntType {
	case 0:
		if _, ok = r.ue(); !ok { // log2_max_pic_order_cnt_lsb_minus4
			return 0, false
		}
	case 1:
		if !skipPicOrderCntType1(r) {
			return 0, false
		}
	}

	maxNumRefFrames, ok = r.ue()
	if !ok {
		return 0, false
	}
	if _, ok = r.u(1); !ok { // gaps_in_frame_num_value_allowed_flag
		return 0, false
	}
	return maxNumRefFrames, true
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

// picSizeFromSPS reads pic_width_in_mbs_minus1 onward and returns both
// the coded picture width and height in pixels - the size FFmpeg's
// own decoder actually allocates frame buffers at, before any
// cropping - and the cropped width and height after
// frame_mbs_only_flag and any frame cropping are applied. ok is also
// false once a crop removes maxCropSlack pixels or more from either
// axis: a real encoder only crops the small macroblock-alignment
// remainder coded dimensions round up to, never more (see
// docs/decisions.md), so a larger crop means the coded and cropped
// sizes were never meant to agree the way isoVideoTrackAllowed's own
// comparison against the container requires.
func picSizeFromSPS(r *bitReader, chromaArrayType uint32) (codedWidth, codedHeight, width, height int, ok bool) {
	picWidthInMbsMinus1, ok := r.ue()
	if !ok {
		return 0, 0, 0, 0, false
	}
	picHeightInMapUnitsMinus1, ok := r.ue()
	if !ok {
		return 0, 0, 0, 0, false
	}
	frameMbsOnlyFlag, ok := readFrameMbsFields(r)
	if !ok {
		return 0, 0, 0, 0, false
	}

	codedWidth = int(picWidthInMbsMinus1+1) * 16
	codedHeight = int((2-frameMbsOnlyFlag)*(picHeightInMapUnitsMinus1+1)) * 16

	width, height, ok = croppedPicSize(r, codedWidth, codedHeight, chromaArrayType, frameMbsOnlyFlag)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return codedWidth, codedHeight, width, height, true
}

// readFrameMbsFields reads frame_mbs_only_flag and, only when it is
// clear, the mb_adaptive_frame_field_flag bit right after it, then
// direct_8x8_inference_flag either way - the fields between the
// picture-size fields and any frame cropping.
func readFrameMbsFields(r *bitReader) (frameMbsOnlyFlag uint32, ok bool) {
	frameMbsOnlyFlag, ok = r.u(1)
	if !ok {
		return 0, false
	}
	if frameMbsOnlyFlag == 0 {
		if _, ok = r.u(1); !ok { // mb_adaptive_frame_field_flag
			return 0, false
		}
	}
	if _, ok = r.u(1); !ok { // direct_8x8_inference_flag
		return 0, false
	}
	return frameMbsOnlyFlag, true
}

// croppedPicSize reads frame cropping and returns the cropped width
// and height, refusing outright once a crop removes maxCropSlack
// pixels or more from either axis: see picSizeFromSPS's own doc
// comment for why.
func croppedPicSize(r *bitReader, codedWidth, codedHeight int, chromaArrayType, frameMbsOnlyFlag uint32) (width, height int, ok bool) {
	cropLeft, cropRight, cropTop, cropBottom, ok := frameCrop(r)
	if !ok {
		return 0, 0, false
	}

	cropUnitX, cropUnitY := frameCropUnits(chromaArrayType, frameMbsOnlyFlag)
	width = codedWidth - int(cropUnitX*(cropLeft+cropRight))
	height = codedHeight - int(cropUnitY*(cropTop+cropBottom))
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}

	maxVerticalSlack := maxCropSlack * int(2-frameMbsOnlyFlag)
	if codedWidth-width >= maxCropSlack || codedHeight-height >= maxVerticalSlack {
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
