package transcode

// avcvalidate.go validates the avc1 and mp4a sample entries a track's
// stsd declares, confirming they describe exactly what this package's
// fixed ffmpeg arguments should have produced before doRemux trusts
// them.

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
)

// spsProfileMain, spsMaxLevel, spsChroma420 and remuxMaxDimension are
// the exact SPS field values checkSPS accepts, matching videoArgs' and
// animationArgs' fixed ffmpeg settings (args.go).
const (
	spsProfileMain    = 77
	spsMaxLevel       = 40
	spsChroma420      = 1
	remuxMaxDimension = 1280
)

// spsMaxCodedMacroblocks bounds the coded picture size, before
// cropping, to level 4.0's MaxFS (ISO/IEC 14496-10 Annex A) - the
// level our fixed ffmpeg invocation always requests.
const spsMaxCodedMacroblocks = 8192

// spsMaxRefFrames bounds num_ref_frames and max_dec_frame_buffering to
// the reference-frame buffer count x264 actually writes with our
// fixed encoder settings, so a crafted SPS cannot inflate the
// decoder's memory use by declaring more.
const spsMaxRefFrames = 4

// spsMaxCropLumaSamples bounds how many luma samples frame cropping
// may remove from the right or bottom edge, so a crafted SPS cannot
// hide a larger coded picture behind a small display size.
const spsMaxCropLumaSamples = 16

// trakStsd returns trak's own sample description box, or nil if its
// mdia/minf/stbl/stsd chain is incomplete.
func trakStsd(trak *mp4.TrakBox) *mp4.StsdBox {
	if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil {
		return nil
	}
	return trak.Mdia.Minf.Stbl.Stsd
}

// trackKind is what classifyTrack found a trak's own stsd to be.
type trackKind int

// trackNeither, trackVideo and trackAudio are classifyTrack's own
// three possible results.
const (
	trackNeither trackKind = iota
	trackVideo
	trackAudio
)

// classifyTrack reports whether trak's own stsd makes it an avc1
// video track, an mp4a audio track, or neither.
func classifyTrack(trak *mp4.TrakBox) trackKind {
	stsd := trakStsd(trak)
	switch {
	case stsd == nil:
		return trackNeither
	case stsd.AvcX != nil:
		return trackVideo
	case stsd.Mp4a != nil:
		return trackAudio
	default:
		return trackNeither
	}
}

// classifyTracks finds exactly one avc1 video track and, when
// allowAudio, at most one mp4a audio track among moov's tracks,
// refusing anything else.
func classifyTracks(moov *mp4.MoovBox, allowAudio bool) (video, audio *mp4.TrakBox, err error) {
	for _, trak := range moov.Traks {
		switch classifyTrack(trak) {
		case trackVideo:
			if video != nil {
				return nil, nil, fmt.Errorf("%w: more than one video track", ErrFailed)
			}
			video = trak
		case trackAudio:
			if audio != nil {
				return nil, nil, fmt.Errorf("%w: more than one audio track", ErrFailed)
			}
			audio = trak
		default:
			return nil, nil, fmt.Errorf("%w: track is neither avc1 nor mp4a", ErrFailed)
		}
	}
	if video == nil {
		return nil, nil, fmt.Errorf("%w: no video track", ErrFailed)
	}
	if audio != nil && !allowAudio {
		return nil, nil, fmt.Errorf("%w: audio track not allowed for this profile", ErrFailed)
	}
	return video, audio, nil
}

// validateVideoTrack confirms trak is exactly one avc1 sample entry
// with one SPS and one PPS, and that both pass their checks.
func validateVideoTrack(trak *mp4.TrakBox) (*mp4.AvcCBox, error) {
	avcX, avcC, err := avcDescriptor(trak)
	if err != nil {
		return nil, err
	}
	if err := parseAndCheckPPS(avcC.PPSnalus[0]); err != nil {
		return nil, err
	}
	if err := parseAndCheckSPS(avcC.SPSnalus[0], avcX.Width, avcX.Height); err != nil {
		return nil, err
	}
	return avcC, nil
}

// avcDescriptor returns trak's single avc1 sample entry and its avcC,
// refusing anything else.
func avcDescriptor(trak *mp4.TrakBox) (*mp4.VisualSampleEntryBox, *mp4.AvcCBox, error) {
	stsd := trakStsd(trak)
	if stsd == nil || len(stsd.Children) != 1 || stsd.AvcX == nil {
		return nil, nil, fmt.Errorf("%w: video track has no single avc1 sample entry", ErrFailed)
	}
	avcX := stsd.AvcX
	avcC := avcX.AvcC
	if avcC == nil || len(avcC.SPSnalus) != 1 || len(avcC.PPSnalus) != 1 {
		return nil, nil, fmt.Errorf("%w: avcC does not have exactly one SPS and one PPS", ErrFailed)
	}
	return avcX, avcC, nil
}

// parseAndCheckPPS parses nalu as a PPS and runs checkPPS on it.
func parseAndCheckPPS(nalu []byte) error {
	pps, err := avc.ParsePPSNALUnit(nalu, nil)
	if err != nil {
		return fmt.Errorf("%w: parsing PPS: %w", ErrFailed, err)
	}
	return checkPPS(pps)
}

// parseAndCheckSPS parses nalu as a SPS, including the VUI's
// bitstream-restriction fields checkSPSRefFrames needs, and runs
// checkSPS against entryWidth and entryHeight.
func parseAndCheckSPS(nalu []byte, entryWidth, entryHeight uint16) error {
	sps, err := avc.ParseSPSNALUnit(nalu, true)
	if err != nil {
		return fmt.Errorf("%w: parsing SPS: %w", ErrFailed, err)
	}
	return checkSPS(sps, entryWidth, entryHeight)
}

// checkPPS checks pps's num_slice_groups_minus1: slice groups need
// extra per-macroblock state our other caps don't bound, so they are
// refused directly rather than relying only on checkSPSCoding's
// Main-profile check.
func checkPPS(pps *avc.PPS) error {
	if pps.NumSliceGroupsMinus1 != 0 {
		return fmt.Errorf("%w: PPS num_slice_groups_minus1 %d, want 0", ErrFailed, pps.NumSliceGroupsMinus1)
	}
	return nil
}

// checkSPS is validateVideoTrack's own field-by-field check,
// separated out so it can be tested directly against hand-built SPS
// values without needing a real H.264 bitstream for every case.
func checkSPS(sps *avc.SPS, entryWidth, entryHeight uint16) error {
	if err := checkSPSCoding(sps); err != nil {
		return err
	}
	if err := checkSPSDimensions(sps, entryWidth, entryHeight); err != nil {
		return err
	}
	return checkSPSRefFrames(sps)
}

// checkSPSCoding checks the coding parameters checkSPS needs beyond
// width and height: AVC Main profile, level 4.0 or below, 4:2:0,
// 8-bit, frame-only.
func checkSPSCoding(sps *avc.SPS) error {
	switch {
	case sps.Profile != spsProfileMain:
		return fmt.Errorf("%w: SPS profile %d, want Main (%d)", ErrFailed, sps.Profile, spsProfileMain)
	case sps.Level > spsMaxLevel:
		return fmt.Errorf("%w: SPS level %d, want at most %d", ErrFailed, sps.Level, spsMaxLevel)
	case sps.ChromaFormatIDC != spsChroma420:
		return fmt.Errorf("%w: SPS chroma format %d, want 4:2:0", ErrFailed, sps.ChromaFormatIDC)
	case sps.BitDepthLumaMinus8 != 0 || sps.BitDepthChromaMinus8 != 0:
		return fmt.Errorf("%w: SPS bit depth is not 8-bit", ErrFailed)
	case !sps.FrameMbsOnlyFlag:
		return fmt.Errorf("%w: SPS is not frame-only", ErrFailed)
	default:
		return nil
	}
}

// checkSPSDimensions checks sps's cropped display size against
// entryWidth/entryHeight and remuxMaxDimension, and also checks the
// uncropped coded picture underneath it: the decoder allocates
// reference frames at the larger coded size, so a crafted SPS can't
// hide a huge coded picture behind a small display size.
func checkSPSDimensions(sps *avc.SPS, entryWidth, entryHeight uint16) error {
	cropUnitX, cropUnitY := cropUnits(sps)
	codedWidth := sps.Width + (sps.FrameCropLeftOffset+sps.FrameCropRightOffset)*cropUnitX
	codedHeight := sps.Height + (sps.FrameCropTopOffset+sps.FrameCropBottomOffset)*cropUnitY
	if err := checkSPSCoded(codedWidth, codedHeight); err != nil {
		return err
	}
	if err := checkSPSCrop(sps); err != nil {
		return err
	}

	switch {
	case sps.Width%2 != 0 || sps.Height%2 != 0:
		return fmt.Errorf("%w: SPS dimensions %dx%d are not even", ErrFailed, sps.Width, sps.Height)
	case sps.Width > remuxMaxDimension || sps.Height > remuxMaxDimension:
		return fmt.Errorf("%w: SPS dimensions %dx%d exceed %d", ErrFailed, sps.Width, sps.Height, remuxMaxDimension)
	case uint16(sps.Width) != entryWidth || uint16(sps.Height) != entryHeight:
		return fmt.Errorf("%w: SPS size %dx%d does not match the sample entry's own %dx%d",
			ErrFailed, sps.Width, sps.Height, entryWidth, entryHeight)
	default:
		return nil
	}
}

// cropUnits returns the horizontal and vertical units sps's
// frame-cropping offsets are measured in (ISO/IEC 14496-10 Section
// 7.4.2.1.1); mp4ff computes this internally but doesn't expose it.
func cropUnits(sps *avc.SPS) (x, y uint) {
	frameMbsOnly := uint(0)
	if sps.FrameMbsOnlyFlag {
		frameMbsOnly = 1
	}
	switch sps.ChromaFormatIDC {
	case 0:
		return 1, 2 - frameMbsOnly
	case 1:
		return 2, 2 * (2 - frameMbsOnly)
	case 2:
		return 2, 1 * (2 - frameMbsOnly)
	default:
		return 1, 1 * (2 - frameMbsOnly)
	}
}

// checkSPSCoded checks the uncropped coded picture size the decoder
// actually allocates reference frames at, rather than the cropped
// display size checkSPSDimensions checks.
func checkSPSCoded(codedWidth, codedHeight uint) error {
	switch {
	case codedWidth%16 != 0 || codedHeight%16 != 0:
		return fmt.Errorf("%w: coded size %dx%d is not a whole number of macroblocks", ErrFailed, codedWidth, codedHeight)
	case codedWidth > remuxMaxDimension || codedHeight > remuxMaxDimension:
		return fmt.Errorf("%w: coded size %dx%d exceeds %d", ErrFailed, codedWidth, codedHeight, remuxMaxDimension)
	case (codedWidth/16)*(codedHeight/16) > spsMaxCodedMacroblocks:
		return fmt.Errorf("%w: coded size %dx%d is %d macroblocks, more than %d allowed",
			ErrFailed, codedWidth, codedHeight, (codedWidth/16)*(codedHeight/16), spsMaxCodedMacroblocks)
	default:
		return nil
	}
}

// checkSPSCrop checks sps's frame-cropping offsets, converted to luma
// samples: x264 only ever crops the right and bottom edges, by less
// than one macroblock.
func checkSPSCrop(sps *avc.SPS) error {
	cropUnitX, cropUnitY := cropUnits(sps)
	left := sps.FrameCropLeftOffset * cropUnitX
	top := sps.FrameCropTopOffset * cropUnitY
	right := sps.FrameCropRightOffset * cropUnitX
	bottom := sps.FrameCropBottomOffset * cropUnitY

	switch {
	case left != 0 || top != 0:
		return fmt.Errorf("%w: SPS crops its left or top edge", ErrFailed)
	case right >= spsMaxCropLumaSamples || bottom >= spsMaxCropLumaSamples:
		return fmt.Errorf("%w: SPS crops %d/%d luma samples from its right/bottom edge, want less than %d",
			ErrFailed, right, bottom, spsMaxCropLumaSamples)
	default:
		return nil
	}
}

// checkSPSRefFrames checks sps's own num_ref_frames and, when the
// VUI declares bitstream restrictions, its max_dec_frame_buffering,
// against spsMaxRefFrames.
func checkSPSRefFrames(sps *avc.SPS) error {
	if sps.NumRefFrames > spsMaxRefFrames {
		return fmt.Errorf("%w: SPS num_ref_frames %d exceeds %d", ErrFailed, sps.NumRefFrames, spsMaxRefFrames)
	}
	if sps.VUI != nil && sps.VUI.BitstreamRestrictionFlag && sps.VUI.MaxDecFrameBuffering > spsMaxRefFrames {
		return fmt.Errorf("%w: SPS max_dec_frame_buffering %d exceeds %d", ErrFailed, sps.VUI.MaxDecFrameBuffering, spsMaxRefFrames)
	}
	return nil
}

// validateAudioTrack confirms trak is exactly one mp4a sample entry
// whose esds declares AAC-LC with an AudioSpecificConfig this
// package can trust (see checkAudioSpecificConfig).
func validateAudioTrack(trak *mp4.TrakBox) (*aac.AudioSpecificConfig, error) {
	stsd := trakStsd(trak)
	if stsd == nil || len(stsd.Children) != 1 || stsd.Mp4a == nil {
		return nil, fmt.Errorf("%w: audio track has no single mp4a sample entry", ErrFailed)
	}
	mp4a := stsd.Mp4a
	decConfig, err := esdsDecConfig(mp4a.Esds)
	if err != nil {
		return nil, err
	}

	asc, err := aac.DecodeAudioSpecificConfig(bytes.NewReader(decConfig))
	if err != nil {
		return nil, fmt.Errorf("%w: parsing AudioSpecificConfig: %w", ErrFailed, err)
	}
	if err := checkAudioSpecificConfig(asc, mp4a.SampleRate); err != nil {
		return nil, err
	}
	return asc, nil
}

// esdsDecConfig returns esds's own AAC AudioSpecificConfig bytes,
// refusing anything that is not exactly an MPEG-4 AAC audio stream
// carrying one.
func esdsDecConfig(esds *mp4.EsdsBox) ([]byte, error) {
	if esds == nil || esds.DecConfigDescriptor == nil {
		return nil, fmt.Errorf("%w: mp4a has no esds decoder config", ErrFailed)
	}
	dcd := esds.DecConfigDescriptor
	if dcd.ObjectType != mp4.ObjectTypeAAC {
		return nil, fmt.Errorf("%w: esds object type %#x is not MPEG-4 AAC", ErrFailed, dcd.ObjectType)
	}
	if dcd.DecSpecificInfo == nil || len(dcd.DecSpecificInfo.DecConfig) == 0 {
		return nil, fmt.Errorf("%w: esds has no AudioSpecificConfig", ErrFailed)
	}
	return dcd.DecSpecificInfo.DecConfig, nil
}

// checkAudioSpecificConfig is validateAudioTrack's own field-by-
// field check, separated out so it can be tested directly.
func checkAudioSpecificConfig(asc *aac.AudioSpecificConfig, entryRate uint16) error {
	channels := aac.NrChannels(asc.ChannelConfiguration)
	switch {
	case asc.ObjectType != aac.AAClc:
		return fmt.Errorf("%w: AAC object type %d, want AAC-LC (%d)", ErrFailed, asc.ObjectType, aac.AAClc)
	case asc.SamplingFrequency != 44100 && asc.SamplingFrequency != 48000:
		return fmt.Errorf("%w: AAC sample rate %d, want 44100 or 48000", ErrFailed, asc.SamplingFrequency)
	case channels != 1 && channels != 2:
		return fmt.Errorf("%w: AAC channel configuration %d, want 1 or 2 channels", ErrFailed, asc.ChannelConfiguration)
	case uint16(asc.SamplingFrequency) != entryRate:
		return fmt.Errorf("%w: AAC sample rate %d does not match the sample entry's own %d",
			ErrFailed, asc.SamplingFrequency, entryRate)
	default:
		return nil
	}
}
