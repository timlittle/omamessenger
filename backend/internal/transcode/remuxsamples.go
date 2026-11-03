package transcode

// remuxsamples.go reads and validates each sample of a track
// remux.go's doRemux is rebuilding, writing the bytes this package
// itself trusts - a video sample's own slice NAL units, filtered; an
// audio sample's bytes, unchanged - to a scratch file on disk rather
// than holding them in memory, so a long or adversarial track costs
// this package disk space, never RAM.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
)

// mp4Source bundles a parsed intermediate file with the read-seeker
// its lazy mdat decoding needs to actually fetch a sample's bytes,
// so passing both around together is one argument, not two.
type mp4Source struct {
	file *mp4.File
	rs   io.ReadSeeker
}

// remuxTrack is one track's own validated shape: everything
// writeRemuxed needs to build its moov and copy its bytes. idr is
// nil for an audio track. chunkOffset is left at zero until
// writeRemuxed assigns it, once it knows where in the output file
// this track's one chunk will actually start.
type remuxTrack struct {
	durations   []uint32
	sizes       []uint32
	idr         []bool
	timescale   uint32
	length      int64
	chunkOffset int64
}

// collectSamples validates every sample of video (and, when present,
// audio) and writes their trusted bytes, in order, to a fresh
// scratch file at scratchPath: video's own filtered NAL units first,
// then audio's own unchanged bytes. Both share one scratchWriter, so
// caps.maxOutputBytes bounds their combined total, not each one
// separately.
func collectSamples(src mp4Source, videoTrak, audioTrak *mp4.TrakBox, scratchPath string, caps remuxCaps) (video, audio *remuxTrack, err error) {
	scratch, err := os.OpenFile(scratchPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: creating scratch file: %w", ErrFailed, err)
	}
	sw := &scratchWriter{f: scratch, max: caps.maxOutputBytes}

	video, err = collectVideoSamples(src, videoTrak, sw, caps.maxVideoSamples, caps.maxVideoSampleLen)
	if err != nil {
		_ = scratch.Close() // write side already failing; doRemux reports the real error and removes the file
		return nil, nil, err
	}

	if audioTrak != nil {
		audio, err = collectAudioSamples(src, audioTrak, sw, caps.maxAudioSamples, caps.maxAudioSampleLen)
		if err != nil {
			_ = scratch.Close() // same as above
			return nil, nil, err
		}
	}

	if err := scratch.Close(); err != nil {
		return nil, nil, fmt.Errorf("%w: closing scratch file: %w", ErrFailed, err)
	}
	return video, audio, nil
}

// scratchWriter writes each validated sample to collectSamples' own
// shared scratch file, refusing to grow video's and audio's combined
// total past max: trakSampleMetadata and readOneSample only ever
// bound one declared sample at a time, so without this, a crafted
// sample table naming many thousands of samples that all alias the
// same handful of bytes in the sandboxed file's own mdat - each
// still under its own per-sample cap - could make this package write
// that small, already-size-capped payload out again once per
// declared sample (see remuxCaps.maxOutputBytes and
// docs/decisions.md).
type scratchWriter struct {
	f     *os.File
	total int64
	max   int64
}

// write appends sample to w's own scratch file, refusing it - before
// anything is actually written - once doing so would take w's own
// running total past max.
func (w *scratchWriter) write(sample []byte) error {
	next := w.total + int64(len(sample))
	if next > w.max {
		return fmt.Errorf("%w: rebuilt output would reach %d bytes, more than %d allowed", ErrFailed, next, w.max)
	}
	if _, err := w.f.Write(sample); err != nil {
		return fmt.Errorf("%w: writing scratch file: %w", ErrFailed, err)
	}
	w.total = next
	return nil
}

// trakSampleMetadata returns trak's own declared samples (duration,
// size, composition time offset), refusing a track with none at all
// or more than maxSamples: a single absurd declared count must cost
// this package no more than checking it costs.
func trakSampleMetadata(trak *mp4.TrakBox, maxSamples int) ([]mp4.Sample, error) {
	n := trak.GetNrSamples()
	if n == 0 {
		return nil, fmt.Errorf("%w: track has no samples", ErrFailed)
	}
	if n > uint32(maxSamples) {
		return nil, fmt.Errorf("%w: track declares %d samples, more than %d allowed", ErrFailed, n, maxSamples)
	}
	return trak.GetSampleData(1, n)
}

// readOneSample fetches sample nr's own bytes from trak through
// src's own lazy mdat reader, refusing anything past maxLen before
// the read even happens: GetRangesForSampleInterval reports a
// declared size without touching mdat, so an absurd one is caught
// before this package asks ReadSampleData to satisfy it.
func readOneSample(src mp4Source, trak *mp4.TrakBox, nr uint32, maxLen int64) ([]byte, error) {
	ranges, err := trak.GetRangesForSampleInterval(nr, nr)
	if err != nil {
		return nil, fmt.Errorf("%w: sample %d: %w", ErrFailed, nr, err)
	}
	var size uint64
	for _, r := range ranges {
		size += r.Size
	}
	if int64(size) > maxLen {
		return nil, fmt.Errorf("%w: sample %d is %d bytes, more than %d allowed", ErrFailed, nr, size, maxLen)
	}

	data, err := src.file.ReadSampleData(trak, nr, nr, src.rs, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: reading sample %d: %w", ErrFailed, nr, err)
	}
	return data, nil
}

// collectVideoSamples validates and filters every sample of trak
// (see processVideoSample) and appends the kept bytes to sw.
func collectVideoSamples(src mp4Source, trak *mp4.TrakBox, sw *scratchWriter, maxSamples int, maxSampleLen int64) (*remuxTrack, error) {
	samples, err := trakSampleMetadata(trak, maxSamples)
	if err != nil {
		return nil, err
	}

	track := &remuxTrack{timescale: trak.Mdia.Mdhd.Timescale}
	for i, s := range samples {
		filtered, isIDR, err := processVideoSample(src, trak, s, i, maxSampleLen)
		if err != nil {
			return nil, err
		}
		if err := sw.write(filtered); err != nil {
			return nil, err
		}
		track.durations = append(track.durations, s.Dur)
		track.sizes = append(track.sizes, uint32(len(filtered)))
		track.idr = append(track.idr, isIDR)
		track.length += int64(len(filtered))
	}
	return track, nil
}

// processVideoSample validates one video sample's own metadata and
// NAL content (see filterVideoSample), refusing a zero duration, a
// nonzero composition time offset, or - for the first sample only -
// anything but an IDR.
func processVideoSample(src mp4Source, trak *mp4.TrakBox, s mp4.Sample, index int, maxSampleLen int64) ([]byte, bool, error) {
	if s.Dur == 0 {
		return nil, false, fmt.Errorf("%w: video sample %d has zero duration", ErrFailed, index+1)
	}
	if s.CompositionTimeOffset != 0 {
		return nil, false, fmt.Errorf("%w: video sample %d has a nonzero composition time offset", ErrFailed, index+1)
	}
	raw, err := readOneSample(src, trak, uint32(index+1), maxSampleLen)
	if err != nil {
		return nil, false, err
	}
	filtered, isIDR, err := filterVideoSample(raw)
	if err != nil {
		return nil, false, err
	}
	if index == 0 && !isIDR {
		return nil, false, fmt.Errorf("%w: first video sample is not an IDR", ErrFailed)
	}
	return filtered, isIDR, nil
}

// filterVideoSample walks sample's own length-prefixed NAL units,
// keeping only slice data (type 1 or 5) and dropping SEI (6) and AUD
// (9), and refuses anything else at all - another in-band SPS or
// PPS included - along with a sample that keeps nothing. It reports
// whether what it kept included an IDR slice.
func filterVideoSample(sample []byte) ([]byte, bool, error) {
	nalus, err := avc.GetNalusFromSample(sample)
	if err != nil {
		return nil, false, fmt.Errorf("%w: reading NAL units: %w", ErrFailed, err)
	}

	var kept [][]byte
	isIDR := false
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			return nil, false, fmt.Errorf("%w: empty NAL unit", ErrFailed)
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_NON_IDR:
			kept = append(kept, nalu)
		case avc.NALU_IDR:
			kept = append(kept, nalu)
			isIDR = true
		case avc.NALU_SEI, avc.NALU_AUD:
			continue
		default:
			return nil, false, fmt.Errorf("%w: unexpected NAL unit type %d", ErrFailed, avc.GetNaluType(nalu[0]))
		}
	}
	if len(kept) == 0 {
		return nil, false, fmt.Errorf("%w: sample has no slice data after filtering", ErrFailed)
	}
	return encodeNALUSample(kept), isIDR, nil
}

// encodeNALUSample rebuilds one sample's own bytes from nalus, each
// prefixed with its own 4-byte big-endian length - the same framing
// avcC's own NAL length size fixes at 4 throughout this package.
func encodeNALUSample(nalus [][]byte) []byte {
	size := 0
	for _, n := range nalus {
		size += 4 + len(n)
	}
	out := make([]byte, 0, size)
	for _, n := range nalus {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(n)))
		out = append(out, length[:]...)
		out = append(out, n...)
	}
	return out
}

// collectAudioSamples validates every sample of trak - nonzero
// duration, no composition time offset, within maxSampleLen - and
// appends its bytes, unchanged, to sw.
func collectAudioSamples(src mp4Source, trak *mp4.TrakBox, sw *scratchWriter, maxSamples int, maxSampleLen int64) (*remuxTrack, error) {
	samples, err := trakSampleMetadata(trak, maxSamples)
	if err != nil {
		return nil, err
	}

	track := &remuxTrack{timescale: trak.Mdia.Mdhd.Timescale}
	for i, s := range samples {
		if s.Dur == 0 {
			return nil, fmt.Errorf("%w: audio sample %d has zero duration", ErrFailed, i+1)
		}
		if s.CompositionTimeOffset != 0 {
			return nil, fmt.Errorf("%w: audio sample %d has a nonzero composition time offset", ErrFailed, i+1)
		}
		raw, err := readOneSample(src, trak, uint32(i+1), maxSampleLen)
		if err != nil {
			return nil, err
		}
		if err := sw.write(raw); err != nil {
			return nil, err
		}
		track.durations = append(track.durations, s.Dur)
		track.sizes = append(track.sizes, uint32(len(raw)))
		track.length += int64(len(raw))
	}
	return track, nil
}
