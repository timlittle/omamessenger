package videosafe

// isobmff.go walks an MP4/MOV (ISO base media file format) box tree
// far enough to answer Validate's question and no further: it never
// reads a box's sample data, only the moov/trak/mdia/minf/stbl boxes
// that name a track's handler, codec, dimensions, duration and sample
// count. Every box body a caller does not ask for is skipped by
// arithmetic on its declared size, never read.

import "encoding/binary"

// isoBMFFPlayable reports whether c's file is a playable MP4/MOV
// under this package's limits: one moov box holding at least one
// video track, every video track within maxVideoTracks using an
// allowed codec and within the dimension and sample-count limits, any
// audio track using an allowed codec, and an overall duration (the
// longest of the movie's own mvhd and every track's own mdhd) within
// maxDuration.
func isoBMFFPlayable(c *cursor) bool {
	moov, ok := c.firstChild(0, c.size, "moov")
	if !ok {
		return false
	}

	duration, _ := c.boxDuration(moov.off, moov.len, "mvhd")

	videoTracks := 0
	sawValidVideo := false
	for _, trak := range c.children(moov.off, moov.len, "trak") {
		kind, codec, width, height, sampleCount, trackDuration, ok := c.trackInfo(trak.off, trak.len)
		if !ok {
			return false
		}
		if trackDuration > duration {
			duration = trackDuration
		}

		isVideo, allowed := isoTrackAllowed(isoTrack{kind: kind, codec: codec, width: width, height: height, sampleCount: sampleCount}, videoTracks)
		if !allowed {
			return false
		}
		if isVideo {
			videoTracks++
			sawValidVideo = true
		}
	}

	return sawValidVideo && duration > 0 && duration <= maxDuration.Seconds()
}

// isoTrack is one track's own handler, codec, dimensions and sample
// count, already read by trackInfo, for isoTrackAllowed to judge.
type isoTrack struct {
	kind        string
	codec       string
	width       int
	height      int
	sampleCount uint32
}

// isoTrackAllowed judges t: isVideo reports whether t.kind is "vide"
// (so the caller counts it toward maxVideoTracks), and ok reports
// whether it passes every limit that applies to its own kind - an
// allowed codec and, for a video track, its dimensions and sample
// count - or allows it through unjudged for any kind that is
// neither, such as a hint or subtitle track this package has no
// opinion on at all.
func isoTrackAllowed(t isoTrack, videoTracks int) (isVideo, ok bool) {
	switch t.kind {
	case "vide":
		if videoTracks >= maxVideoTracks || !allowedISOVideoCodec[t.codec] {
			return true, false
		}
		if t.width <= 0 || t.height <= 0 || t.width > maxDimension || t.height > maxDimension {
			return true, false
		}
		return true, t.sampleCount <= maxSampleCount
	case "soun":
		return false, allowedISOAudioCodec[t.codec]
	default:
		return false, true
	}
}

// trackInfo extracts trak's handler type ("vide" or "soun"), its
// first sample entry's codec fourcc, a video track's declared width
// and height, its declared sample count (from stsz/stz2, read as a
// single field, never allocated) and its own duration in seconds
// (from mdhd). ok is false once any box this needs is missing or too
// short to read, so a track whose header was cut off is never
// silently treated as safe.
func (c *cursor) trackInfo(trakOff, trakLen int64) (kind, codec string, width, height int, sampleCount uint32, duration float64, ok bool) {
	mdia, found := c.firstChild(trakOff, trakLen, "mdia")
	if !found {
		return
	}

	kind, ok = c.handlerType(mdia.off, mdia.len)
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

	codec, ok = c.sampleEntryCodec(stbl.off, stbl.len)
	if !ok {
		return
	}

	if kind == "vide" {
		width, height, ok = c.sampleEntryDims(stbl.off, stbl.len)
		if !ok {
			return
		}
	}

	sampleCount, ok = c.sampleCount(stbl.off, stbl.len)
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
// [containerOff, containerOff+containerLen); it stops, returning
// whatever it already found, at the first malformed box, the same way
// a truncated file naturally yields fewer tracks than it claims.
func (c *cursor) children(containerOff, containerLen int64, want string) []span {
	limit := containerOff + containerLen
	var out []span
	for off := containerOff; off < limit; {
		typ, body, next, ok := c.boxHeader(off, limit)
		if !ok {
			return out
		}
		if typ == want {
			out = append(out, body)
		}
		off = next
	}
	return out
}
