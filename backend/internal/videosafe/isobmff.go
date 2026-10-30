package videosafe

// isobmff.go walks an MP4/MOV (ISO base media file format) box tree
// far enough to answer Validate's question and no further: mostly the
// moov/trak/mdia/minf/stbl boxes that name a track's handler, codec,
// dimensions, duration and sample count, never a box's actual sample
// data - except for an avc1 video track's own samples, which
// avcVideoTrackChecks (h264sps.go) and avcsamples.go read far enough
// to refuse an in-band parameter set, the one piece of sample data
// this package cannot safely leave unread (see docs/decisions.md).
// Every other box body a caller does not ask for is skipped by
// arithmetic on its declared size, never read.

import "encoding/binary"

// isoBMFFPlayable reports whether c's file is a playable MP4/MOV
// under this package's limits: no fragmented-MP4 moof/mfra box
// anywhere at the top level, one moov box holding exactly the tracks
// this package has validated - one video track within the dimension
// and sample-count limits, and at most one audio track using an
// allowed codec, no other track of any kind - and an overall duration
// (the longest of the movie's own mvhd and every track's own mdhd)
// within maxDuration.
func isoBMFFPlayable(c *cursor) bool {
	moov, ok := c.topLevelMoov()
	if !ok {
		return false
	}

	duration, _ := c.boxDuration(moov.off, moov.len, "mvhd")

	traks, ok := c.children(moov.off, moov.len, "trak")
	if !ok {
		return false
	}

	videoTracks, audioTracks := 0, 0
	sawValidVideo := false
	for _, trak := range traks {
		track, trackDuration, ok := c.trackInfo(trak.off, trak.len)
		if !ok {
			return false
		}
		if trackDuration > duration {
			duration = trackDuration
		}

		isVideo, isAudio, allowed := isoTrackAllowed(track, videoTracks, audioTracks)
		if !allowed {
			return false
		}
		if isVideo {
			videoTracks++
			sawValidVideo = true
		}
		if isAudio {
			audioTracks++
		}
	}

	return sawValidVideo && duration > 0 && duration <= maxDuration.Seconds()
}

// topLevelMoov walks every top-level box in the file once, both to
// find moov and to refuse a fragmented MP4 outright: a moof or mfra
// box anywhere means sample data lives in additional movie fragments
// this package never reads, which FFmpeg's own mov demuxer still
// opens and decodes (see docs/decisions.md), so a file carrying
// either is never reported playable regardless of what its moov box
// alone declares. ok is false once a top-level box is malformed, the
// same "cannot read it, cannot trust it" rule every other check in
// this package already applies.
func (c *cursor) topLevelMoov() (moov span, ok bool) {
	var found bool
	for off := int64(0); off < c.size; {
		typ, body, next, headerOK := c.boxHeader(off, c.size)
		if !headerOK {
			return span{}, false
		}
		if typ == "moof" || typ == "mfra" {
			return span{}, false
		}
		if typ == "moov" && !found {
			moov, found = body, true
		}
		off = next
	}
	return moov, found
}

// isoTrack is one track's own handler, codec, dimensions, declared
// SPS dimensions and sample count, already read by trackInfo, for
// isoTrackAllowed to judge.
type isoTrack struct {
	kind        string
	codec       string
	width       int
	height      int
	spsWidth    int
	spsHeight   int
	spsOK       bool
	inBandOK    bool
	sampleCount uint32
}

// isoTrackAllowed judges t: isVideo and isAudio report which of the
// caller's own counters to advance, and ok reports whether t passes
// every limit that applies to its own kind. A track of any other
// kind - hint, subtitle, timed metadata, or anything else - is
// refused outright: FFmpeg's own mov demuxer opens and decodes every
// track whose codec fourcc it recognizes regardless of the track's
// own declared handler type (see docs/decisions.md), so a track this
// package has not itself judged is never safe to let through
// unjudged, the way it once was.
func isoTrackAllowed(t isoTrack, videoTracks, audioTracks int) (isVideo, isAudio, ok bool) {
	switch t.kind {
	case "vide":
		return true, false, isoVideoTrackAllowed(t, videoTracks)
	case "soun":
		return false, true, audioTracks < maxAudioTracks && allowedISOAudioCodec[t.codec]
	default:
		return false, false, false
	}
}

// isoVideoTrackAllowed reports whether a video track passes every
// limit this package applies to one: an allowed codec, a
// container-declared width and height within the cap, and - since
// the container's own fields are never what the decoder actually
// uses - a fully parseable SPS (see h264sps.go) whose own coded
// width and height agree with those container fields exactly. A
// mismatch, or an SPS this package could not parse at all, is
// refused rather than trusted either way. Agreeing with the avcC SPS
// is still not enough on its own: inBandOK additionally requires that
// none of the track's own samples carry a second, untrusted parameter
// set FFmpeg would still honor (see avcsamples.go and
// docs/decisions.md) - the exact gap a differential security review
// found this package's SPS check alone did not close.
func isoVideoTrackAllowed(t isoTrack, videoTracks int) bool {
	if videoTracks >= maxVideoTracks || !allowedISOVideoCodec[t.codec] {
		return false
	}
	if t.width <= 0 || t.height <= 0 || t.width > maxDimension || t.height > maxDimension {
		return false
	}
	if !t.spsOK || t.spsWidth != t.width || t.spsHeight != t.height {
		return false
	}
	if !t.inBandOK {
		return false
	}
	return t.sampleCount <= maxSampleCount
}

// trackInfo extracts trak's handler type ("vide" or "soun"), its
// first sample entry's codec fourcc, a video track's declared width
// and height, its codec configuration's own declared width and
// height (avcC's SPS, for an avc1 track, plus the in-band sample scan
// avcVideoTrackChecks runs for it), its declared sample count (from
// stsz/stz2, read as a single field, never allocated outside that
// scan) and its own duration in seconds (from mdhd). ok is false once
// any box this needs is missing or too short to read, so a track
// whose header was cut off is never silently treated as safe.
func (c *cursor) trackInfo(trakOff, trakLen int64) (track isoTrack, duration float64, ok bool) {
	mdia, found := c.firstChild(trakOff, trakLen, "mdia")
	if !found {
		return
	}

	track.kind, ok = c.handlerType(mdia.off, mdia.len)
	if !ok {
		return
	}

	duration, ok = c.boxDuration(mdia.off, mdia.len, "mdhd")
	if !ok {
		return
	}

	minf, found := c.firstChild(mdia.off, mdia.len, "minf")
	if !found {
		ok = false
		return
	}
	stbl, found := c.firstChild(minf.off, minf.len, "stbl")
	if !found {
		ok = false
		return
	}

	track.codec, ok = c.sampleEntryCodec(stbl.off, stbl.len)
	if !ok {
		return
	}
	track.sampleCount, ok = c.sampleCount(stbl.off, stbl.len)
	if !ok {
		return
	}

	track.inBandOK = true
	if track.kind != "vide" {
		return
	}
	track.width, track.height, ok = c.sampleEntryDims(stbl.off, stbl.len)
	if !ok || track.codec != "avc1" {
		return
	}
	track.spsWidth, track.spsHeight, track.spsOK, track.inBandOK, ok = c.avcVideoTrackChecks(stbl, track.sampleCount)
	return
}

// handlerType reads mdia's hdlr box, reporting the track's handler
// type ("vide", "soun", or anything else this package does not care
// about), the four bytes at a fixed offset in every hdlr box
// regardless of version.
func (c *cursor) handlerType(mdiaOff, mdiaLen int64) (string, bool) {
	hdlr, found := c.firstChild(mdiaOff, mdiaLen, "hdlr")
	if !found || hdlr.len < 12 {
		return "", false
	}

	var buf [12]byte
	if !c.readAt(buf[:], hdlr.off) {
		return "", false
	}

	return string(buf[8:12]), true
}

// boxDuration reads a duration-carrying box's own timescale and
// duration fields: mvhd (movie-level) and mdhd (track-level) share
// the exact same layout up to those two fields, which is all this
// package ever needs from either one.
func (c *cursor) boxDuration(containerOff, containerLen int64, boxType string) (float64, bool) {
	box, found := c.firstChild(containerOff, containerLen, boxType)
	if !found {
		return 0, false
	}

	var version [1]byte
	if !c.readAt(version[:], box.off) {
		return 0, false
	}

	if version[0] == 1 {
		if box.len < 32 {
			return 0, false
		}
		var buf [32]byte
		if !c.readAt(buf[:], box.off) {
			return 0, false
		}
		return durationSeconds(binary.BigEndian.Uint32(buf[20:24]), binary.BigEndian.Uint64(buf[24:32])), true
	}

	if box.len < 20 {
		return 0, false
	}
	var buf [20]byte
	if !c.readAt(buf[:], box.off) {
		return 0, false
	}
	return durationSeconds(binary.BigEndian.Uint32(buf[12:16]), uint64(binary.BigEndian.Uint32(buf[16:20]))), true
}

// durationSeconds converts a timescale (units per second) and a
// duration (in those units) to seconds, reporting 0 for an unknown
// (zero) timescale rather than dividing by it.
func durationSeconds(timescale uint32, duration uint64) float64 {
	if timescale == 0 {
		return 0
	}
	return float64(duration) / float64(timescale)
}

// sampleEntryCodec reads stbl's stsd box far enough to return its
// first sample entry's own four-byte type - the codec fourcc every
// check in this package reads, never a sender's mime type or file
// extension.
func (c *cursor) sampleEntryCodec(stblOff, stblLen int64) (string, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 16 {
		return "", false
	}

	var buf [16]byte
	if !c.readAt(buf[:], stsd.off) {
		return "", false
	}

	return string(buf[12:16]), true
}

// sampleEntryDims reads a video stsd box's first sample entry far
// enough to return its declared width and height: fixed fields of
// ISO/IEC 14496-12's VisualSampleEntry, 24 bytes into the entry's own
// body, which itself starts 16 bytes into stsd (4 bytes of
// version/flags, 4 of entry_count, then the entry's own 8-byte
// size+type header).
func (c *cursor) sampleEntryDims(stblOff, stblLen int64) (int, int, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 44 {
		return 0, 0, false
	}

	var buf [44]byte
	if !c.readAt(buf[:], stsd.off) {
		return 0, 0, false
	}

	return int(binary.BigEndian.Uint16(buf[40:42])), int(binary.BigEndian.Uint16(buf[42:44])), true
}

// sampleCount reads stbl's stsz or stz2 box for its declared
// sample_count field alone, at the same 8-byte offset in either box,
// never the per-sample entries that may follow it.
func (c *cursor) sampleCount(stblOff, stblLen int64) (uint32, bool) {
	for _, boxType := range []string{"stsz", "stz2"} {
		box, found := c.firstChild(stblOff, stblLen, boxType)
		if !found || box.len < 12 {
			continue
		}

		var buf [12]byte
		if !c.readAt(buf[:], box.off) {
			continue
		}

		return binary.BigEndian.Uint32(buf[8:12]), true
	}

	return 0, false
}

// boxHeader reads the box starting at off, within the container
// bounded by limit, returning its type, body and the offset right
// after it (for the next sibling). ok is false once the box is
// malformed, extends past limit, or this parse has already visited
// too many boxes (see maxBoxBudget), so a crafted chain of boxes can
// never make this loop run for long.
func (c *cursor) boxHeader(off, limit int64) (typ string, body span, next int64, ok bool) {
	if c.boxes <= 0 || off+8 > limit {
		return "", span{}, 0, false
	}
	c.boxes--

	var hdr [8]byte
	if !c.readAt(hdr[:], off) {
		return "", span{}, 0, false
	}

	size := int64(binary.BigEndian.Uint32(hdr[:4]))
	typ = string(hdr[4:8])
	headerLen := int64(8)

	switch size {
	case 0:
		size = limit - off
	case 1:
		if off+16 > limit {
			return "", span{}, 0, false
		}
		var ext [8]byte
		if !c.readAt(ext[:], off+8) {
			return "", span{}, 0, false
		}
		size = int64(binary.BigEndian.Uint64(ext[:]))
		headerLen = 16
	}

	// Comparing size against the small, bounded limit-off rather than
	// off+size avoids a signed overflow: a crafted 64-bit size near
	// the top of its range would otherwise wrap off+size negative and
	// slip past a naive check.
	if size < headerLen || size > limit-off {
		return "", span{}, 0, false
	}

	return typ, span{off: off + headerLen, len: size - headerLen}, off + size, true
}

// firstChild returns the first immediate child of the given type
// within [containerOff, containerOff+containerLen), or ok false when
// there is none or the container itself is malformed.
func (c *cursor) firstChild(containerOff, containerLen int64, want string) (span, bool) {
	limit := containerOff + containerLen
	for off := containerOff; off < limit; {
		typ, body, next, ok := c.boxHeader(off, limit)
		if !ok {
			return span{}, false
		}
		if typ == want {
			return body, true
		}
		off = next
	}
	return span{}, false
}

// children returns every immediate child of the given type within
// [containerOff, containerOff+containerLen). ok is false once any
// box in that range - matching or not - is malformed: FFmpeg's own
// mov demuxer shrinks an oversized child box to fit its parent rather
// than stop there, and still parses the track that follows it, so
// this package must judge every track the file declares, never
// silently treat a malformed one partway through as "no more tracks"
// (see docs/decisions.md).
func (c *cursor) children(containerOff, containerLen int64, want string) ([]span, bool) {
	limit := containerOff + containerLen
	var out []span
	for off := containerOff; off < limit; {
		typ, body, next, ok := c.boxHeader(off, limit)
		if !ok {
			return nil, false
		}
		if typ == want {
			out = append(out, body)
		}
		off = next
	}
	return out, true
}
