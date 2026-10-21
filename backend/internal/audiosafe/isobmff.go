package audiosafe

// isobmff.go walks an MP4/M4A (ISO base media file format) box tree
// far enough to answer Validate's question and no further: it never
// reads a box's sample data, only the moov/trak/mdia/minf/stbl boxes
// that name a track's handler, codec and duration. This is a
// deliberately smaller, audio-only sibling of videosafe's own box
// walker rather than a shared import: videosafe's cursor, span and
// box-reading helpers are unexported, and the two packages judge
// different things (a video track's dimensions and sample count
// here never matter at all, since a video track here is an outright
// rejection, not a track to measure).

import "encoding/binary"

// validateISOBMFFAudio reports whether c's file is a playable
// MP4/M4A under this package's limits: one moov box holding exactly
// one audio track in an allowed codec, no video track at all, and a
// duration (the longest of the movie's own mvhd and the audio
// track's own mdhd) within maxDuration.
func validateISOBMFFAudio(c *cursor) bool {
	moov, ok := c.firstChild(0, c.size, "moov")
	if !ok {
		return false
	}

	duration, _ := c.boxDuration(moov.off, moov.len, "mvhd")

	audioTracks := 0
	for _, trak := range c.children(moov.off, moov.len, "trak") {
		kind, codec, trackDuration, ok := c.audioTrackInfo(trak.off, trak.len)
		if !ok {
			return false
		}
		if trackDuration > duration {
			duration = trackDuration
		}

		isAudio, allowed := isoAudioTrackAllowed(kind, codec, audioTracks)
		if !allowed {
			return false
		}
		if isAudio {
			audioTracks++
		}
	}

	return audioTracks == 1 && duration > 0 && duration <= maxDuration.Seconds()
}

// isoAudioTrackAllowed judges one track's handler type and codec,
// the same shape videosafe's own isoTrackAllowed judges a video
// track: a video track ("vide") is always rejected outright, since
// no audio file this package accepts may carry one at all; an audio
// track ("soun") is allowed only the first time, in one of
// allowedISOAudioCodec's own codecs; any other kind (hint, subtitle,
// ...) passes through unjudged, this package having no opinion on it.
func isoAudioTrackAllowed(kind, codec string, audioTracksSoFar int) (isAudio, ok bool) {
	switch kind {
	case "vide":
		return false, false
	case "soun":
		return true, audioTracksSoFar == 0 && allowedISOAudioCodec[codec]
	default:
		return false, true
	}
}

// audioTrackInfo extracts trak's handler type ("vide" or "soun"),
// its first sample entry's codec fourcc, and its own duration in
// seconds (from mdhd). ok is false once any box this needs is
// missing or too short to read, so a track whose header was cut off
// is never silently treated as safe.
func (c *cursor) audioTrackInfo(trakOff, trakLen int64) (kind, codec string, duration float64, ok bool) {
	mdia, found := c.firstChild(trakOff, trakLen, "mdia")
	if !found {
		return "", "", 0, false
	}

	kind, ok = c.handlerType(mdia.off, mdia.len)
	if !ok {
		return "", "", 0, false
	}

	duration, ok = c.boxDuration(mdia.off, mdia.len, "mdhd")
	if !ok {
		return "", "", 0, false
	}

	if kind != "soun" {
		return kind, "", duration, true
	}

	minf, found := c.firstChild(mdia.off, mdia.len, "minf")
	if !found {
		return "", "", 0, false
	}
	stbl, found := c.firstChild(minf.off, minf.len, "stbl")
	if !found {
		return "", "", 0, false
	}

	codec, ok = c.sampleEntryCodec(stbl.off, stbl.len)
	return kind, codec, duration, ok
}

// handlerType reads mdia's hdlr box, reporting the track's handler
// type ("vide", "soun", or anything else this package does not care
// about), the four bytes at a fixed offset in every hdlr box
// regardless of version - the same layout videosafe's own
// handlerType reads.
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
// the exact same layout up to those two fields, the same reasoning
// videosafe's own boxDuration documents.
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
// first sample entry's own four-byte type - the codec fourcc this
// package reads, never a sender's mime type or file extension.
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
	// off+size avoids a signed overflow, the same reasoning videosafe's
	// own boxHeader documents.
	if size < headerLen || size > limit-off {
		return "", span{}, 0, false
	}

	return typ, span{off: off + headerLen, len: size - headerLen}, off + size, true
}

// span is a box's body: offset and length, both counted from the
// start of the file.
type span struct {
	off int64
	len int64
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
// whatever it already found, at the first malformed box, the same
// way a truncated file naturally yields fewer tracks than it claims.
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
