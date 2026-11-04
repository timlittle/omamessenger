package transcode

// remuxbuild.go builds the moov and ftyp for doRemux's rebuilt file
// and copies the scratch file's validated bytes into place.

import (
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// mdatHeaderBytes is the ordinary 8-byte mdat header size: every
// profile's cap keeps the payload well under the 32-bit size limit.
const mdatHeaderBytes = 8

// videoTrackID and audioTrackID are the fixed track IDs given to the
// one video and, when present, one audio track this package writes.
const (
	videoTrackID = 1
	audioTrackID = 2
)

// remuxDescriptors bundles the validated sample descriptors buildMoov
// builds fresh avc1 and mp4a sample entries from.
type remuxDescriptors struct {
	avcC *mp4.AvcCBox
	asc  *aac.AudioSpecificConfig
}

// writeRemuxed builds the moov for video (and audio, when present)
// and writes ftyp, moov and mdat to outPath, copying scratchPath's
// bytes into mdat without holding them in memory at once.
func writeRemuxed(outPath, scratchPath string, descriptors remuxDescriptors, video, audio *remuxTrack) error {
	moov, err := buildMoov(descriptors, video, audio)
	if err != nil {
		return err
	}
	ftyp := mp4.NewFtyp(mp4.BrandIsom, 0, []string{mp4.BrandIso2, mp4.BrandAvc1, mp4.BrandMp41})

	dataStart := int64(ftyp.Size()) + int64(moov.Size()) + mdatHeaderBytes
	video.chunkOffset = dataStart
	payload := video.length
	if audio != nil {
		audio.chunkOffset = dataStart + video.length
		payload += audio.length
	}
	assignChunkOffsets(moov, video, audio)

	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%w: creating output: %w", ErrFailed, err)
	}

	writeErr := encodeOutput(out, ftyp, moov, scratchPath, payload)
	closeErr := out.Close()
	switch {
	case writeErr != nil:
		return writeErr
	case closeErr != nil:
		return fmt.Errorf("%w: closing output: %w", ErrFailed, closeErr)
	default:
		return nil
	}
}

// assignChunkOffsets writes video's and, when present, audio's
// chunkOffset into the stco box buildMoov already created; this runs
// after moov.Size() since filling the value does not change the size.
func assignChunkOffsets(moov *mp4.MoovBox, video, audio *remuxTrack) {
	moov.Traks[0].Mdia.Minf.Stbl.Stco.ChunkOffset[0] = uint32(video.chunkOffset)
	if audio != nil {
		moov.Traks[1].Mdia.Minf.Stbl.Stco.ChunkOffset[0] = uint32(audio.chunkOffset)
	}
}

// buildMoov assembles a fresh moov for video (and, when present,
// audio) from already-validated descriptors, never copying a sample
// entry from the file being rebuilt.
func buildMoov(descriptors remuxDescriptors, video, audio *remuxTrack) (*mp4.MoovBox, error) {
	moov := mp4.NewMoovBox()
	moov.AddChild(mp4.CreateMvhd())

	videoTrak, err := buildVideoTrak(video, descriptors.avcC)
	if err != nil {
		return nil, err
	}
	moov.AddChild(videoTrak)

	movieDur := sumDurations(video.durations)
	if audio != nil {
		audioTrak, err := buildAudioTrak(audio, descriptors.asc)
		if err != nil {
			return nil, err
		}
		moov.AddChild(audioTrak)
		if audioDur := rescaleTicks(sumDurations(audio.durations), audio.timescale, video.timescale); audioDur > movieDur {
			movieDur = audioDur
		}
	}

	moov.Mvhd.Timescale = video.timescale
	moov.Mvhd.Duration = uint64(movieDur)
	moov.Mvhd.NextTrackID = uint32(len(moov.Traks) + 1)
	return moov, nil
}

// buildVideoTrak returns a fresh trak for video, with its avcC built
// only from srcAvcC's validated SPS and PPS bytes.
func buildVideoTrak(video *remuxTrack, srcAvcC *mp4.AvcCBox) (*mp4.TrakBox, error) {
	trak := mp4.CreateEmptyTrak(videoTrackID, video.timescale, "video", "und")
	if err := trak.SetAVCDescriptor("avc1", srcAvcC.SPSnalus, srcAvcC.PPSnalus, true); err != nil {
		return nil, fmt.Errorf("%w: building avc1 descriptor: %w", ErrFailed, err)
	}
	fillSampleTables(trak, video, buildStss(video.idr))
	trak.Tkhd.Duration = uint64(sumDurations(video.durations))
	trak.Mdia.Mdhd.Duration = trak.Tkhd.Duration
	return trak, nil
}

// buildAudioTrak returns a fresh trak for audio, with its esds built
// only from asc's validated fields. Audio has no stss: every AAC-LC
// frame decodes on its own.
func buildAudioTrak(audio *remuxTrack, asc *aac.AudioSpecificConfig) (*mp4.TrakBox, error) {
	trak := mp4.CreateEmptyTrak(audioTrackID, audio.timescale, "audio", "und")
	if err := trak.SetAACDescriptorWithChannelConfig(asc.ObjectType, asc.ChannelConfiguration, asc.SamplingFrequency); err != nil {
		return nil, fmt.Errorf("%w: building mp4a descriptor: %w", ErrFailed, err)
	}
	fillSampleTables(trak, audio, nil)
	trak.Tkhd.Duration = uint64(sumDurations(audio.durations))
	trak.Mdia.Mdhd.Duration = trak.Tkhd.Duration
	return trak, nil
}

// fillSampleTables replaces trak's empty stts/stsc/stsz/stco (and
// stss, when non-nil) with ones built from track's validated samples.
// chunkOffset is filled in later, once the output layout is known.
func fillSampleTables(trak *mp4.TrakBox, track *remuxTrack, stss *mp4.StssBox) {
	stbl := trak.Mdia.Minf.Stbl
	stbl.Stts = buildStts(track.durations)
	stbl.Stsc = &mp4.StscBox{}
	_ = stbl.Stsc.AddEntry(1, uint32(len(track.sizes)), 1) // a fixed entry; can never fail validation
	stbl.Stsz = buildStsz(track.sizes)
	stbl.Stco = &mp4.StcoBox{ChunkOffset: []uint32{0}}
	stbl.Stss = stss
	stbl.Children = sampleTableChildren(stbl)
}

// sampleTableChildren orders stbl's children as a stbl box requires:
// stsd, stts, stsc, stsz, stss (when present), stco.
func sampleTableChildren(stbl *mp4.StblBox) []mp4.Box {
	children := []mp4.Box{stbl.Stsd, stbl.Stts, stbl.Stsc, stbl.Stsz}
	if stbl.Stss != nil {
		children = append(children, stbl.Stss)
	}
	return append(children, stbl.Stco)
}

// buildStts run-length encodes durs into a fresh stts box.
func buildStts(durs []uint32) *mp4.SttsBox {
	stts := &mp4.SttsBox{}
	for _, d := range durs {
		if n := len(stts.SampleCount); n > 0 && stts.SampleTimeDelta[n-1] == d {
			stts.SampleCount[n-1]++
			continue
		}
		stts.SampleCount = append(stts.SampleCount, 1)
		stts.SampleTimeDelta = append(stts.SampleTimeDelta, d)
	}
	return stts
}

// buildStsz lists sizes verbatim; these tracks are too short for
// uniform-size compaction to be worth the extra code.
func buildStsz(sizes []uint32) *mp4.StszBox {
	return &mp4.StszBox{SampleNumber: uint32(len(sizes)), SampleSize: sizes}
}

// buildStss lists the 1-based sample numbers idr marks true.
func buildStss(idr []bool) *mp4.StssBox {
	stss := &mp4.StssBox{}
	for i, v := range idr {
		if v {
			stss.SampleNumber = append(stss.SampleNumber, uint32(i+1))
		}
	}
	return stss
}

// sumDurations adds up durs as an int64; the duration and sample-count
// caps keep the total well under overflow.
func sumDurations(durs []uint32) int64 {
	var total int64
	for _, d := range durs {
		total += int64(d)
	}
	return total
}

// rescaleTicks converts dur ticks at timescale from into the
// equivalent tick count at timescale to, rounding to the nearest tick.
func rescaleTicks(dur int64, from, to uint32) int64 {
	if from == to || from == 0 {
		return dur
	}
	return (dur*int64(to) + int64(from)/2) / int64(from)
}

// encodeOutput writes ftyp, moov and an mdat header sized for
// payloadLen, then copies exactly payloadLen bytes from scratchPath
// into it.
func encodeOutput(out io.Writer, ftyp *mp4.FtypBox, moov *mp4.MoovBox, scratchPath string, payloadLen int64) error {
	if err := ftyp.Encode(out); err != nil {
		return fmt.Errorf("%w: encoding ftyp: %w", ErrFailed, err)
	}
	if err := moov.Encode(out); err != nil {
		return fmt.Errorf("%w: encoding moov: %w", ErrFailed, err)
	}
	if err := mp4.EncodeHeaderWithSize("mdat", uint64(payloadLen)+mdatHeaderBytes, false, out); err != nil {
		return fmt.Errorf("%w: encoding mdat header: %w", ErrFailed, err)
	}
	return copyScratch(out, scratchPath, payloadLen)
}

// copyScratch copies exactly wantLen bytes from scratchPath to out,
// in bounded pieces rather than reading the whole scratch file into
// memory at once.
func copyScratch(out io.Writer, scratchPath string, wantLen int64) error {
	scratch, err := os.Open(scratchPath)
	if err != nil {
		return fmt.Errorf("%w: reopening scratch file: %w", ErrFailed, err)
	}
	defer func() {
		_ = scratch.Close() // read-only handle: nothing left to report once the copy below has already returned its own error
	}()

	if _, err := io.CopyN(out, scratch, wantLen); err != nil {
		return fmt.Errorf("%w: copying scratch file: %w", ErrFailed, err)
	}
	return nil
}
