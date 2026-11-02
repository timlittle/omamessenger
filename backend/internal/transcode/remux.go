package transcode

// remux.go rebuilds an MP4 the sandboxed ffmpeg wrote into a brand
// new file this package writes itself, with mp4ff: every box in the
// sandboxed file is read only to extract the handful of values a
// canonical, minimal MP4 needs, each validated before use, and
// nothing it contains - not a byte of its own moov, not a NAL unit
// this package does not itself recognise as video slice data - ever
// crosses into the rebuilt file uninspected. A sandboxed ffmpeg that
// is fully compromised can then still only ever reach Qt's own
// H.264 and AAC decoders, never anything that parses a container,
// because the container itself always one this package wrote. See
// docs/decisions.md for the exact shape this writes and why.

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
)

// spsProfileMain, spsMaxLevel, spsChroma420 and remuxMaxDimension are
// the exact SPS field values checkSPS accepts: AVC Main profile,
// level 4.0 or below, 4:2:0 chroma, no larger than 1280 on either
// side - precisely what videoArgs' and animationArgs' own fixed
// ffmpeg settings produce (see args.go).
const (
	spsProfileMain    = 77
	spsMaxLevel       = 40
	spsChroma420      = 1
	remuxMaxDimension = 1280
)

// remuxSampleCountSlack gives a profile's own duration-cap-derived
// sample count a little room: fpsmax and a fixed AAC frame rate cap
// the encoder, not the input's own frame timing exactly, so a source
// a hair over the cap can still land a sample or two past the
// nominal count.
const remuxSampleCountSlack = 60

// maxAACFramesPerSecond bounds how many AAC-LC frames per second of
// audio this package expects: generous for both sample rates it
// accepts (1024-sample frames give about 43/sec at 44.1 kHz, 47/sec
// at 48 kHz).
const maxAACFramesPerSecond = 50

// maxRemuxVideoSampleBytes bounds any one video sample this package
// will rebuild: videoRateControlArgs' own VBV bufsize (5000 kbit,
// see args.go) already bounds how much any single frame can burst to
// under the encoder's own rate control, with headroom to spare.
const maxRemuxVideoSampleBytes = 4 << 20

// maxRemuxAudioSampleBytes bounds any one AAC frame a remuxed
// video's own audio track may carry: generous for even the highest
// bitrate this package would ever ask ffmpeg to produce.
const maxRemuxAudioSampleBytes = 32 << 10

// maxMoovBytes bounds a moov box's own declared size, checked
// before mp4.DecodeFile ever reads it: decoding a moov box reads its
// entire payload into memory to decode its own tables (stts, stsz,
// stco and the rest), and mp4ff's own per-table checks bound each
// table to its own box's declared size, never the box itself. A
// crafted intermediate could otherwise make that one read cost
// nearly as much as checkOutput's own whole-file cap allows, before
// any of this package's own structural checks run at all. Measured
// directly: Video's own worst case - ten minutes at 1280x720/30fps
// with 48 kHz AAC, the longest and most densely sampled file this
// package's own ffmpeg arguments ever legitimately produce - has a
// 519,147-byte moov; 2 MiB leaves roughly 4x that as headroom.
const maxMoovBytes = 2 << 20

// maxTopLevelBoxes bounds how many top-level boxes
// checkTopLevelBoxes will walk before refusing: an ordinary
// faststart MP4 this package's own ffmpeg arguments produce has four
// (ftyp, moov, free, mdat). This leaves headroom for minor variation
// without letting a crafted file force an unbounded walk.
const maxTopLevelBoxes = 8

// checkTopLevelBoxes walks f's own top-level boxes from its current
// position to the file's end, reading only each box's own header
// (mp4.DecodeHeader) and seeking past its body without ever reading
// it, so this stays cheap regardless of what a hostile file claims.
// It refuses more than one moov box, a moov box over maxMoovBytes,
// more than maxTopLevelBoxes boxes in total, or any box whose own
// declared size reaches past the file's actual end. f is left
// positioned wherever the walk stopped; openIntermediate rewinds it
// before using it for anything else.
func checkTopLevelBoxes(f io.ReadSeeker) error {
	fileSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("%w: finding file size: %w", ErrFailed, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("%w: rewinding to walk top-level boxes: %w", ErrFailed, err)
	}

	moovCount := 0
	boxes := 0
	for pos := int64(0); pos < fileSize; boxes++ {
		if boxes >= maxTopLevelBoxes {
			return fmt.Errorf("%w: more than %d top-level boxes", ErrFailed, maxTopLevelBoxes)
		}

		header, err := mp4.DecodeHeader(f)
		if err != nil {
			return fmt.Errorf("%w: reading top-level box header: %w", ErrFailed, err)
		}
		if err := checkBoxFitsFile(header, pos, fileSize); err != nil {
			return err
		}
		if err := checkMoovSize(header, &moovCount); err != nil {
			return err
		}

		if _, err := f.Seek(int64(header.Size)-int64(header.Hdrlen), io.SeekCurrent); err != nil {
			return fmt.Errorf("%w: skipping box %q: %w", ErrFailed, header.Name, err)
		}
		pos += int64(header.Size)
	}
	return nil
}

// checkBoxFitsFile refuses header when it claims to reach past
// fileSize. The comparison stays entirely in uint64: header.Size can
// claim any 64-bit value (mdat's own large-size form allows one),
// and converting that down to int64 first could wrap negative and
// let a too-large claim slip past a signed comparison instead of
// being caught by it.
func checkBoxFitsFile(header mp4.BoxHeader, pos, fileSize int64) error {
	remaining := uint64(fileSize - pos)
	if header.Size > remaining {
		return fmt.Errorf("%w: box %q size %d runs past the file's own end", ErrFailed, header.Name, header.Size)
	}
	return nil
}

// checkMoovSize updates moovCount for header and refuses a second
// moov box, or one over maxMoovBytes.
func checkMoovSize(header mp4.BoxHeader, moovCount *int) error {
	if header.Name != "moov" {
		return nil
	}
	*moovCount++
	if *moovCount > 1 {
		return fmt.Errorf("%w: more than one moov box", ErrFailed)
	}
	if header.Size > maxMoovBytes {
		return fmt.Errorf("%w: moov box is %d bytes, more than %d allowed", ErrFailed, header.Size, maxMoovBytes)
	}
	return nil
}

// remuxCaps bounds what remuxMP4 is willing to accept from the
// sandboxed file it rebuilds, sized per profile (see video.go and
// animation.go). Neither bound is this package's real defence
// against an oversized output - checkOutput's own cap on the
// sandboxed file as a whole, enforced before remuxMP4 ever runs,
// already provides that - they exist only so a single absurd value
// (a billion declared samples, one claiming to be a gigabyte) cannot
// cost this package more time or memory than parsing a file already
// known to be small should.
type remuxCaps struct {
	allowAudio        bool
	maxVideoSamples   int
	maxAudioSamples   int
	maxVideoSampleLen int64
	maxAudioSampleLen int64
}

// remuxMP4 validates path - an MP4 the sandboxed ffmpeg wrote -
// against caps, then writes a new file at outPath holding only what
// it validated: one avc1 video track, AVC Main profile, with at most
// one mp4a AAC-LC audio track when caps.allowAudio allows it. Any
// failure, including a panic while parsing path (mp4ff decodes
// untrusted input; it is not expected to panic, but nothing here
// trusts that it cannot), is reported as ErrFailed.
func remuxMP4(path, outPath string, caps remuxCaps) (err error) {
	defer func() {
		// The recovered value itself is never logged or wrapped: it
		// can carry whatever string a panicking line happened to
		// build, which may itself echo bytes from the untrusted file
		// being parsed. %T names only the panic's own type, never its
		// value (see tools/nologcontent and docs/decisions.md).
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: panic rebuilding container (%T)", ErrFailed, r)
		}
	}()
	return doRemux(path, outPath, caps)
}

// doRemux is remuxMP4's own body, split out so remuxMP4's recover
// covers every return path through one deferred call.
func doRemux(path, outPath string, caps remuxCaps) error {
	in, parsed, err := openIntermediate(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close() // read-only handle: nothing left to report that matters once the checks below have already returned their own error
	}()

	videoTrak, audioTrak, err := classifyTracks(parsed.Moov, caps.allowAudio)
	if err != nil {
		return err
	}
	avcC, err := validateVideoTrack(videoTrak)
	if err != nil {
		return err
	}
	var asc *aac.AudioSpecificConfig
	if audioTrak != nil {
		if asc, err = validateAudioTrack(audioTrak); err != nil {
			return err
		}
	}

	scratchPath := outPath + ".scratch"
	src := mp4Source{file: parsed, rs: in}
	video, audio, err := collectSamples(src, videoTrak, audioTrak, scratchPath, caps)
	if err != nil {
		_ = os.Remove(scratchPath) // best-effort: doRemux is already returning the real error
		return err
	}
	defer func() {
		_ = os.Remove(scratchPath) // the scratch file's job ends the moment writeRemuxed has read it back
	}()

	return writeRemuxed(outPath, scratchPath, remuxDescriptors{avcC: avcC, asc: asc}, video, audio)
}

// openIntermediate opens path, checks its top-level box structure
// (checkTopLevelBoxes) and only then parses it as an MP4 with
// mp4ff's own lazy mdat decoding, so a large sample payload - or an
// oversized moov, which mp4ff reads whole regardless of decode mode -
// is never read into memory just to find out how many tracks the
// file has.
func openIntermediate(path string) (*os.File, *mp4.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: opening intermediate: %w", ErrFailed, err)
	}

	if err := checkTopLevelBoxes(f); err != nil {
		_ = f.Close() // read-only handle already failing; nothing left to report that matters
		return nil, nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: rewinding intermediate: %w", ErrFailed, err)
	}

	parsed, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		_ = f.Close() // read-only handle already failing to parse; nothing left to report that matters
		return nil, nil, fmt.Errorf("%w: parsing intermediate: %w", ErrFailed, err)
	}
	if parsed.Moov == nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: no moov box", ErrFailed)
	}
	return f, parsed, nil
}

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
// allowAudio, at most one mp4a audio track among moov's own tracks,
// refusing anything else: a second video track, any audio track at
// all when allowAudio is false, or a track that is neither.
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
// over an avcC with exactly one SPS and one PPS at NAL length size 4
// (DecodeAVCDecConfRec itself refuses any other length size), whose
// SPS and PPS both parse and whose SPS passes checkSPS.
func validateVideoTrack(trak *mp4.TrakBox) (*mp4.AvcCBox, error) {
	stsd := trakStsd(trak)
	if stsd == nil || len(stsd.Children) != 1 || stsd.AvcX == nil {
		return nil, fmt.Errorf("%w: video track has no single avc1 sample entry", ErrFailed)
	}
	avcX := stsd.AvcX
	avcC := avcX.AvcC
	if avcC == nil || len(avcC.SPSnalus) != 1 || len(avcC.PPSnalus) != 1 {
		return nil, fmt.Errorf("%w: avcC does not have exactly one SPS and one PPS", ErrFailed)
	}
	if _, err := avc.ParsePPSNALUnit(avcC.PPSnalus[0], nil); err != nil {
		return nil, fmt.Errorf("%w: parsing PPS: %w", ErrFailed, err)
	}

	sps, err := avc.ParseSPSNALUnit(avcC.SPSnalus[0], false)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing SPS: %w", ErrFailed, err)
	}
	if err := checkSPS(sps, avcX.Width, avcX.Height); err != nil {
		return nil, err
	}
	return avcC, nil
}

// checkSPS is validateVideoTrack's own field-by-field check,
// separated out so it can be tested directly against hand-built SPS
// values without needing a real H.264 bitstream for every case.
func checkSPS(sps *avc.SPS, entryWidth, entryHeight uint16) error {
	if err := checkSPSCoding(sps); err != nil {
		return err
	}
	return checkSPSDimensions(sps, entryWidth, entryHeight)
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

// checkSPSDimensions checks sps's own width and height: even, no
// larger than remuxMaxDimension on either side, and equal to the
// sample entry's own declared entryWidth and entryHeight.
func checkSPSDimensions(sps *avc.SPS, entryWidth, entryHeight uint16) error {
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
