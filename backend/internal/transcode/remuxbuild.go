package transcode

// remuxbuild.go builds the moov and ftyp remux.go's doRemux writes
// for its rebuilt file, and copies its scratch file's already-
// validated bytes into place: see remux.go for what is validated
// before any of this runs.

import (
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// mdatHeaderBytes is the ordinary 8-byte mdat header size this
// package always gets: every profile's own cap keeps a rebuilt
// file's total payload far under the 4 GiB a plain 32-bit size field
// can hold.
const mdatHeaderBytes = 8

// videoTrackID and audioTrackID are the fixed track IDs this package
// gives the one video and, when present, one audio track it ever
// writes.
const (
	videoTrackID = 1
	audioTrackID = 2
)

// remuxDescriptors bundles the two already-validated sample
// descriptors buildMoov builds a fresh avc1 and, when audio is
// present, mp4a sample entry from - one argument instead of two, to
// keep writeRemuxed's own parameter count down.
type remuxDescriptors struct {
	avcC *mp4.AvcCBox
	asc  *aac.AudioSpecificConfig
}

// writeRemuxed builds the moov for video (and audio, when present),
// then writes ftyp, moov and mdat to outPath, copying scratchPath's
// own bytes - video's samples, then audio's - into mdat with a
// bounded copy rather than holding them in memory at once.
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

// assignChunkOffsets writes video's and, when present, audio's own
// chunkOffset into the stco box buildMoov already gave each track:
// moov's own size does not change when a stco slot's value changes,
// only when the slot is added, so this can run after moov.Size() has
// already been used to compute those very offsets.
func assignChunkOffsets(moov *mp4.MoovBox, video, audio *remuxTrack) {
	moov.Traks[0].Mdia.Minf.Stbl.Stco.ChunkOffset[0] = uint32(video.chunkOffset)
	if audio != nil {
		moov.Traks[1].Mdia.Minf.Stbl.Stco.ChunkOffset[0] = uint32(audio.chunkOffset)
	}
}

// buildMoov assembles a fresh moov for video (and, when present,
// audio), using mp4ff's own sample-entry builders fed only the
// values remux.go already validated: a fresh avcC from
// descriptors.avcC's own already-validated SPS and PPS bytes, and,
// for audio, a fresh esds from descriptors.asc. Neither sample entry
// is copied from the file being rebuilt.
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

// buildVideoTrak returns a fresh trak for video, with its own avcC
// built from srcAvcC's already-validated SPS and PPS bytes (never
// srcAvcC itself, so nothing else it might carry crosses over) and
// its sample tables built from video's own validated samples.
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

// buildAudioTrak returns a fresh trak for audio, with its own esds
// built from asc's already-validated fields (never the file's own
// esds) and its sample tables built from audio's own validated
// samples. Audio has no stss: every AAC-LC frame decodes on its own.
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

// fillSampleTables replaces trak's own empty stts/stsc/stsz/stco
// (and, when stss is non-nil, adds it too) with ones built from
// track's own validated samples. track's chunkOffset is not known
// yet at this point; buildMoov's own caller fills it in once moov's
// size - and so the output file's layout - is known.
func fillSampleTables(trak *mp4.TrakBox, track *remuxTrack, stss *mp4.StssBox) {
	stbl := trak.Mdia.Minf.Stbl
	stbl.Stts = buildStts(track.durations)
	stbl.Stsc = &mp4.StscBox{}
	_ = stbl.Stsc.AddEntry(1, uint32(len(track.sizes)), 1) // a fixed, always-valid entry: a first chunk of 1 can never fail validation
	stbl.Stsz = buildStsz(track.sizes)
	stbl.Stco = &mp4.StcoBox{ChunkOffset: []uint32{0}}
	stbl.Stss = stss
	stbl.Children = sampleTableChildren(stbl)
}

// sampleTableChildren returns stbl's own children in the fixed
// order a stbl box needs, given its own fields already set: stsd
// first, then stts, stsc, stsz, stss (when present) and stco.
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

// buildStsz lists sizes verbatim: this package's own tracks are
// short enough that the uniform-size compaction a larger file
// benefits from is not worth the extra code.
func buildStsz(sizes []uint32) *mp4.StszBox {
	return &mp4.StszBox{SampleNumber: uint32(len(sizes)), SampleSize: sizes}
}

// buildStss lists the 1-based sample numbers idr marks true: exactly
// the IDR frames this package's own NAL filtering already found.
func buildStss(idr []bool) *mp4.StssBox {
	stss := &mp4.StssBox{}
	for i, v := range idr {
		if v {
			stss.SampleNumber = append(stss.SampleNumber, uint32(i+1))
		}
	}
	return stss
}

// sumDurations adds up durs as an int64: every individual duration
// and this package's own sample-count cap keep the total far under
// an int64 overflow for any track this package ever builds.
func sumDurations(durs []uint32) int64 {
	var total int64
	for _, d := range durs {
		total += int64(d)
	}
	return total
}

// rescaleTicks converts dur ticks at timescale from into the
// equivalent tick count at timescale to, rounding to the nearest
// tick. Every duration this package ever rescales is already capped
// by a profile's own duration limit, so plain int64 arithmetic
// cannot overflow here the way it would need to guard against for
// an arbitrary, unbounded media duration.
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
