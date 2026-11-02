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
// MP4/M4A under this package's limits: no fragmented-MP4 moof/mfra
// box anywhere at the top level, no iTunes-style cover art atom (see
// hasCoverArtAtom), one moov box holding exactly one audio track in
// an allowed codec and no other track of any kind, and a duration
// (the longest of the movie's own mvhd and the audio track's own
// mdhd) within maxDuration.
func validateISOBMFFAudio(c *cursor) bool {
	moov, ok := c.topLevelMoov()
	if !ok {
		return false
	}
	if !c.onlyChildrenAllowed(moov.off, moov.len, allowedMoovChild) {
		return false
	}
	if hasArt, scanOK := c.hasCoverArtAnywhere(); !scanOK || hasArt {
		return false
	}

	duration, _ := c.boxDuration(moov.off, moov.len, "mvhd")

	traks, ok := c.children(moov.off, moov.len, "trak")
	if !ok {
		return false
	}

	audioTracks, duration, ok := c.tallyAudioTracks(traks, duration)
	if !ok {
		return false
	}

	return audioTracks == 1 && duration > 0 && duration <= maxDuration.Seconds()
}

// tallyAudioTracks checks every trak in traks, counting the audio
// tracks and returning the longest of duration and each track's own
// duration; ok is false as soon as one track is unreadable or not an
// allowed audio track.
func (c *cursor) tallyAudioTracks(traks []span, duration float64) (audioTracks int, longest float64, ok bool) {
	longest = duration
	for _, trak := range traks {
		kind, codec, trackDuration, readOK := c.audioTrackInfo(trak.off, trak.len)
		if !readOK {
			return 0, 0, false
		}
		longest = max(longest, trackDuration)

		isAudio, allowed := isoAudioTrackAllowed(kind, codec, audioTracks)
		if !allowed {
			return 0, 0, false
		}
		if isAudio {
			audioTracks++
		}
	}

	return audioTracks, longest, true
}

// topLevelMoov walks every top-level box in the file once, both to
// find moov and to refuse outright anything allowedTopLevelBox does
// not name - a moof or mfra box, or any other box this package does
// not expect there: see videosafe's own topLevelMoov, which this
// mirrors, and docs/decisions.md.
func (c *cursor) topLevelMoov() (moov span, ok bool) {
	var found bool
	for off := int64(0); off < c.size; {
		typ, body, next, headerOK := c.boxHeader(off, c.size)
		if !headerOK || !allowedTopLevelBox[typ] {
			return span{}, false
		}
		if typ == "moov" && !found {
			moov, found = body, true
		}
		off = next
	}
	return moov, found
}

// coverArtContainer reports whether typ is a box type this package
// recurses into while looking for a cover art atom: FFmpeg's own mov
// demuxer reads a 'covr' box out of a 'meta' box found at any of
// several places - directly under moov, under moov > udta, under a
// trak's own udta, or bare at the file's own top level (see
// docs/decisions.md and hasCoverArtAnywhere) - so this package no
// longer assumes only the one, most common placement. Any other box
// type is skipped over by its own declared size, never descended
// into: most of this format's non-container boxes (mdhd, hdlr, stsd,
// and the rest) open with a FullBox version/flags field or other
// fixed data that would misparse as nested box headers if this walk
// ever tried to read into them too.
func coverArtContainer(typ string) bool {
	switch typ {
	case "moov", "trak", "udta", "meta", "ilst":
		return true
	default:
		return false
	}
}

// hasCoverArtAnywhere reports whether the file carries an iTunes-style
// cover art atom ('covr') anywhere at all, recursing into every box
// coverArtContainer names, starting from the file's own top level, so
// there is nowhere left for one to hide among the handful of places
// FFmpeg's own mov demuxer reads one from. ok is false once the walk
// could not finish - a malformed box, or the shared box budget or
// maxCoverArtDepth running out - since an incomplete scan can never be
// trusted to have seen the whole file; found is then meaningless, and
// the caller must refuse the file rather than trust a result this scan
// never actually reached.
func (c *cursor) hasCoverArtAnywhere() (found, ok bool) {
	return c.walkForCoverArt(0, c.size, 0)
}

// walkForCoverArt is hasCoverArtAnywhere's own recursion, over every
// box in [containerOff, containerOff+containerLen), to the given
// nesting depth.
func (c *cursor) walkForCoverArt(containerOff, containerLen int64, depth int) (found, ok bool) {
	if depth > maxCoverArtDepth {
		return false, false
	}

	limit := containerOff + containerLen
	for off := containerOff; off < limit; {
		typ, body, next, headerOK := c.boxHeader(off, limit)
		if !headerOK {
			return false, false
		}
		if typ == "covr" {
			return true, true
		}

		child, recurse := body, coverArtContainer(typ)
		if typ == "meta" {
			// meta is a FullBox: 4 bytes of version/flags precede its
			// own children. Too short to even hold them means nothing
			// could fit inside, not a reason to refuse the file.
			if body.len < 4 {
				recurse = false
			} else {
				child = span{off: body.off + 4, len: body.len - 4}
			}
		}

		if recurse {
			childFound, childOK := c.walkForCoverArt(child.off, child.len, depth+1)
			if !childOK {
				return false, false
			}
			if childFound {
				return true, true
			}
		}

		off = next
	}

	return false, true
}

// isoAudioTrackAllowed judges one track's handler type and codec: a
// video track ("vide") is always rejected outright, since no audio
// file this package accepts may carry one at all; an audio track
// ("soun") is allowed only the first time, in one of
// allowedISOAudioCodec's own codecs. Any other kind - hint, subtitle,
// timed metadata, or anything else - is refused outright: FFmpeg's
// own mov demuxer opens and decodes every track whose codec fourcc it
// recognizes regardless of the track's own declared handler type (see
// docs/decisions.md), so a track this package has not itself judged
// is never safe to let through unjudged, the way it once was.
func isoAudioTrackAllowed(kind, codec string, audioTracksSoFar int) (isAudio, ok bool) {
	switch kind {
	case "vide":
		return false, false
	case "soun":
		return true, audioTracksSoFar == 0 && allowedISOAudioCodec[codec]
	default:
		return false, false
	}
}

// audioTrackInfo extracts trak's handler type ("vide" or "soun"),
// its first sample entry's codec fourcc, and its own duration in
// seconds (from mdhd). ok is false once any box this needs is
// missing, too short to read, or - through onlyChildrenAllowed -
// carries a child box outside this package's own allowlist (see
// docs/decisions.md on the cmov gap this closes), so a track whose
// header was cut off, or whose structure hides an unvalidated box,
// is never silently treated as safe. minf and stbl are only read,
// and so only checked, once kind is "soun": a track of any other
// kind is already refused by the caller regardless of what either
// box holds (see isoAudioTrackAllowed and soundCodec, split out to
// keep this function's own branching within this package's
// complexity limit).
func (c *cursor) audioTrackInfo(trakOff, trakLen int64) (kind, codec string, duration float64, ok bool) {
	if !c.onlyChildrenAllowed(trakOff, trakLen, allowedTrakChild) {
		return "", "", 0, false
	}

	mdia, found := c.firstChild(trakOff, trakLen, "mdia")
	if !found || !c.onlyChildrenAllowed(mdia.off, mdia.len, allowedMdiaChild) {
		return "", "", 0, false
	}

	kind, ok = c.handlerType(mdia.off, mdia.len)
	if !ok {
		return "", "", 0, false
	}

	timescale, rawDuration, ok := c.boxTimescaleDuration(mdia.off, mdia.len, "mdhd")
	if !ok {
		return "", "", 0, false
	}
	duration = durationSeconds(timescale, rawDuration)

	if kind != "soun" {
		return kind, "", duration, true
	}

	codec, ok = c.soundTrackFields(mdia, trakOff, trakLen, timescale)
	return kind, codec, duration, ok
}

// soundTrackFields reads a "soun" track's own minf and stbl boxes,
// checking each against this package's own allowlist as it goes (see
// onlyChildrenAllowed), its codec fourcc, sample count and sample
// tables (see sampleTablesConsistent), its own sample entry's
// children (see audioSampleEntryOK) and its trak-level edit list (see
// editListAllowed): the part of audioTrackInfo that only runs once
// the track's handler type is already known to be "soun". A security
// review found this package read none of these beyond the codec
// fourcc alone - not an audio track's own sample count, any of its
// other sample tables, its edit list, or what sat inside its one
// sample entry besides that fourcc (see docs/decisions.md).
func (c *cursor) soundTrackFields(mdia span, trakOff, trakLen int64, timescale uint32) (string, bool) {
	stbl, ok := c.soundStbl(mdia)
	if !ok {
		return "", false
	}

	codec, ok := c.sampleEntryCodec(stbl.off, stbl.len)
	if !ok {
		return "", false
	}
	if !c.soundTablesAndEntryOK(trakOff, trakLen, stbl, codec, timescale) {
		return "", false
	}
	return codec, true
}

// soundStbl finds mdia's own minf and stbl boxes, checking each
// against this package's own allowlist as it goes: split out of
// soundTrackFields to keep its own branching within this package's
// complexity limit.
func (c *cursor) soundStbl(mdia span) (span, bool) {
	minf, found := c.firstChild(mdia.off, mdia.len, "minf")
	if !found || !c.onlyChildrenAllowed(minf.off, minf.len, allowedMinfChild) {
		return span{}, false
	}
	stbl, found := c.firstChild(minf.off, minf.len, "stbl")
	if !found || !c.onlyChildrenAllowed(stbl.off, stbl.len, allowedStblChild) {
		return span{}, false
	}
	return stbl, true
}

// soundTablesAndEntryOK reports whether the sound track's own sample
// count is within maxSampleCount, its sample tables all agree with
// each other and with that count (see sampleTablesConsistent), its
// own edit list (if any) declares at most one in-range entry, and its
// one sample entry's own children describe a codec this package
// accepts (see audioSampleEntryOK): split out of soundTrackFields to
// keep its own branching within this package's complexity limit.
func (c *cursor) soundTablesAndEntryOK(trakOff, trakLen int64, stbl span, codec string, timescale uint32) bool {
	sampleCount, ok := c.sampleCount(stbl.off, stbl.len)
	if !ok || sampleCount > maxSampleCount {
		return false
	}
	if !c.sampleTablesConsistent(stbl, sampleCount) {
		return false
	}
	if !c.editListAllowed(trakOff, trakLen, timescale) {
		return false
	}
	return c.audioSampleEntryOK(stbl.off, stbl.len, codec)
}

// sampleCount reads stbl's stsz or stz2 box for its declared
// sample_count field alone, at the same 8-byte offset in either box,
// never the per-sample entries that may follow it: mirrors
// videosafe's own sampleCount.
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
// duration fields and converts them to seconds (see
// boxTimescaleDuration): mvhd (movie-level) and mdhd (track-level)
// share the exact same layout up to those two fields, the same
// reasoning videosafe's own boxDuration documents.
func (c *cursor) boxDuration(containerOff, containerLen int64, boxType string) (float64, bool) {
	timescale, duration, ok := c.boxTimescaleDuration(containerOff, containerLen, boxType)
	if !ok {
		return 0, false
	}
	return durationSeconds(timescale, duration), true
}

// boxTimescaleDuration reads a duration-carrying box's own raw
// timescale and duration fields, never converted to seconds: the part
// of boxDuration a caller that also needs the raw timescale (see
// audioTrackInfo's own edit-list check) reuses directly, mirroring
// videosafe's own boxTimescaleDuration.
func (c *cursor) boxTimescaleDuration(containerOff, containerLen int64, boxType string) (timescale uint32, duration uint64, ok bool) {
	box, found := c.firstChild(containerOff, containerLen, boxType)
	if !found {
		return 0, 0, false
	}

	var version [1]byte
	if !c.readAt(version[:], box.off) {
		return 0, 0, false
	}

	if version[0] == 1 {
		if box.len < 32 {
			return 0, 0, false
		}
		var buf [32]byte
		if !c.readAt(buf[:], box.off) {
			return 0, 0, false
		}
		return binary.BigEndian.Uint32(buf[20:24]), binary.BigEndian.Uint64(buf[24:32]), true
	}

	if box.len < 20 {
		return 0, 0, false
	}
	var buf [20]byte
	if !c.readAt(buf[:], box.off) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint32(buf[12:16]), uint64(binary.BigEndian.Uint32(buf[16:20])), true
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
// package reads, never a sender's mime type or file extension. ok is
// also false when stsd's entry_count is anything but 1: this package
// only ever reads the first entry, but FFmpeg's own mov demuxer keeps
// every entry sharing a track's codec tag and can switch to a later
// one's own codec configuration mid-stream, so a second entry would
// never be checked at all - the same gap videosafe's own
// sampleEntryCodec documents and a differential security review
// found; see docs/decisions.md.
func (c *cursor) sampleEntryCodec(stblOff, stblLen int64) (string, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 16 {
		return "", false
	}

	var buf [16]byte
	if !c.readAt(buf[:], stsd.off) {
		return "", false
	}

	if binary.BigEndian.Uint32(buf[4:8]) != 1 {
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
// [containerOff, containerOff+containerLen). ok is false once any
// box in that range - matching or not - is malformed: see videosafe's
// own children, which this mirrors, and docs/decisions.md.
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

// onlyChildrenAllowed reports whether every immediate child within
// [containerOff, containerOff+containerLen) is one of allowed's own
// box types, refusing the container outright the moment one is not:
// see videosafe's own onlyChildrenAllowed, which this mirrors, and
// docs/decisions.md on the cmov gap this closes.
func (c *cursor) onlyChildrenAllowed(containerOff, containerLen int64, allowed map[string]bool) bool {
	limit := containerOff + containerLen
	for off := containerOff; off < limit; {
		typ, _, next, ok := c.boxHeader(off, limit)
		if !ok {
			return false
		}
		if !allowed[typ] {
			return false
		}
		off = next
	}
	return true
}
