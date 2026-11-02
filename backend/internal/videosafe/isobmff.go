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

import (
	"context"
	"encoding/binary"
)

// isoBMFFPlayable reports whether c's file is a playable MP4/MOV
// under this package's limits: no fragmented-MP4 moof/mfra box
// anywhere at the top level, no iTunes-style cover art atom (see
// hasCoverArtAtom), one moov box holding exactly the tracks this
// package has validated - one video track within the dimension and
// sample-count limits, and at most one audio track using an allowed
// codec, no other track of any kind - and an overall duration (the
// longest of the movie's own mvhd and every track's own mdhd) within
// maxDuration. ctx is passed on to addTrak, so a cancelled caller
// does not wait out an avc1 track's own in-band sample scan, the one
// part of this walk whose cost scales with the file rather than a
// small, fixed box budget (see avcsamples.go).
func isoBMFFPlayable(ctx context.Context, c *cursor) bool {
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

	traks, ok := c.children(moov.off, moov.len, "trak")
	if !ok {
		return false
	}

	var tally trackTally
	tally.duration, _ = c.boxDuration(moov.off, moov.len, "mvhd")
	for _, trak := range traks {
		if !c.addTrak(ctx, trak, &tally) {
			return false
		}
	}

	return tally.sawValidVideo && tally.duration > 0 && tally.duration <= maxDuration.Seconds()
}

// trackTally accumulates isoBMFFPlayable's own running state across
// every trak it judges: the longest duration seen so far, and how
// many video and audio tracks have already passed isoTrackAllowed.
type trackTally struct {
	duration                 float64
	videoTracks, audioTracks int
	sawValidVideo            bool
}

// addTrak judges one trak (trackInfo, then isoTrackAllowed) and folds
// the result into t: a longer duration, and one more video or audio
// track counted toward maxVideoTracks/maxAudioTracks. ok is false the
// moment the track cannot be read at all or fails every limit that
// applies to its own kind - isoBMFFPlayable's own cue to refuse the
// whole file - split out from it only to keep its own branching
// within this package's complexity limit.
func (c *cursor) addTrak(ctx context.Context, trak span, t *trackTally) bool {
	track, trackDuration, ok := c.trackInfo(ctx, trak.off, trak.len)
	if !ok {
		return false
	}
	if trackDuration > t.duration {
		t.duration = trackDuration
	}

	isVideo, isAudio, allowed := isoTrackAllowed(track, t.videoTracks, t.audioTracks)
	if !allowed {
		return false
	}
	if isVideo {
		t.videoTracks++
		t.sawValidVideo = true
	}
	if isAudio {
		t.audioTracks++
	}
	return true
}

// topLevelMoov walks every top-level box in the file once, both to
// find moov and to refuse outright anything allowedTopLevelBox does
// not name - a moof or mfra box, meaning sample data lives in
// additional movie fragments this package never reads, or any other
// box FFmpeg's own mov demuxer would still open and decode while this
// package stayed silent about it (see docs/decisions.md) - so a file
// carrying one is never reported playable regardless of what its
// moov box alone declares. ok is false once a top-level box is
// malformed or disallowed, the same "cannot read it, cannot trust
// it" rule every other check in this package already applies.
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
	if !dimensionsWithinCap(t.width, t.height) {
		return false
	}
	if !t.spsOK || t.spsWidth != t.width || t.spsHeight != t.height {
		return false
	}
	return t.inBandOK
}

// dimensionsWithinCap reports whether width and height are both
// positive and no larger than maxDimension: the one check both a
// video track's container-declared size and its SPS-derived one must
// pass.
func dimensionsWithinCap(width, height int) bool {
	return width > 0 && height > 0 && width <= maxDimension && height <= maxDimension
}

// trackInfo extracts trak's handler type ("vide" or "soun"), its
// first sample entry's codec fourcc, a video track's declared width
// and height, its codec configuration's own declared width and
// height (avcC's SPS, for an avc1 track, plus the in-band sample scan
// avcVideoTrackChecks runs for it), its declared sample count (from
// stsz/stz2, read as a single field, never allocated outside that
// scan) and its own duration in seconds (from mdhd). ok is false once
// any box this needs is missing, too short to read, or - through
// trackChildBoxes - carries a child outside this package's own
// allowlist, so a track whose header was cut off, or whose structure
// hides an unvalidated box, is never silently treated as safe.
func (c *cursor) trackInfo(ctx context.Context, trakOff, trakLen int64) (track isoTrack, duration float64, ok bool) {
	mdia, stbl, ok := c.trackChildBoxes(trakOff, trakLen)
	if !ok {
		return
	}

	track.kind, ok = c.handlerType(mdia.off, mdia.len)
	if !ok {
		return
	}

	timescale, rawDuration, ok := c.boxTimescaleDuration(mdia.off, mdia.len, "mdhd")
	if !ok {
		return
	}
	duration = durationSeconds(timescale, rawDuration)

	track.codec, ok = c.sampleEntryCodec(stbl.off, stbl.len)
	if !ok {
		return
	}
	track.sampleCount, ok = c.sampleCount(stbl.off, stbl.len)
	if !ok {
		return
	}
	if !c.trackTablesAndEditsOK(trakOff, trakLen, stbl, track.sampleCount, timescale) {
		ok = false
		return
	}

	track.inBandOK = true
	ok = c.trackKindFieldsOK(ctx, track.kind, stbl, &track)
	return
}

// trackTablesAndEditsOK reports whether a track's own sample count is
// within maxSampleCount, its sample tables (stsz/stz2, stco/co64,
// stsc, stts, ctts, stss, sgpd and sbgp - see sampletables.go) all
// agree with each other and with that count, and its own edit list
// (if any) declares at most one in-range entry - every bound that
// used to apply to the video track alone, through isoVideoTrackAllowed,
// now applied here to every track this package reads regardless of
// kind: a security review found the audio track's own sampleCount,
// read a few lines above, was never checked against anything at all.
func (c *cursor) trackTablesAndEditsOK(trakOff, trakLen int64, stbl span, sampleCount, timescale uint32) bool {
	if sampleCount > maxSampleCount {
		return false
	}
	if !c.sampleTablesConsistent(stbl, sampleCount) {
		return false
	}
	return c.editListAllowed(trakOff, trakLen, timescale)
}

// trackKindFieldsOK reads and validates whatever this package checks
// beyond a track's own codec, sample count and sample tables, specific
// to its kind: a video track's own avcC/SPS agreement and in-band NAL
// scan (see videoTrackFields). A track of neither kind is refused by
// isoTrackAllowed regardless of what this reports, so there is
// nothing to check for it here.
func (c *cursor) trackKindFieldsOK(ctx context.Context, kind string, stbl span, track *isoTrack) bool {
	switch kind {
	case "vide":
		return c.videoTrackFields(ctx, stbl, track)
	default:
		return true
	}
}

// trackChildBoxes walks trak down through mdia, minf and stbl,
// checking each container's own children against this package's
// allowlist as it goes (see onlyChildrenAllowed and docs/decisions.md
// on the cmov gap this closes), so a box FFmpeg's own mov demuxer
// would still parse - a compressed moov, a fragmented MP4's own
// sample tables, or anything else this package does not expect to
// find there - can never hide a track's real structure from the rest
// of trackInfo. ok is false the moment any expected box is missing,
// any container's own children fail the allowlist, or a box itself
// is malformed.
func (c *cursor) trackChildBoxes(trakOff, trakLen int64) (mdia, stbl span, ok bool) {
	if !c.onlyChildrenAllowed(trakOff, trakLen, allowedTrakChild) {
		return span{}, span{}, false
	}

	mdia, found := c.firstChild(trakOff, trakLen, "mdia")
	if !found || !c.onlyChildrenAllowed(mdia.off, mdia.len, allowedMdiaChild) {
		return span{}, span{}, false
	}

	minf, found := c.firstChild(mdia.off, mdia.len, "minf")
	if !found || !c.onlyChildrenAllowed(minf.off, minf.len, allowedMinfChild) {
		return span{}, span{}, false
	}

	stbl, found = c.firstChild(minf.off, minf.len, "stbl")
	if !found || !c.onlyChildrenAllowed(stbl.off, stbl.len, allowedStblChild) {
		return span{}, span{}, false
	}

	return mdia, stbl, true
}

// videoTrackFields fills a video track's own declared width and
// height and, for an avc1 codec, every field avcVideoTrackChecks adds
// (spsWidth, spsHeight, spsOK, inBandOK): the part of trackInfo that
// only applies once a track's own kind is "vide", split out to keep
// trackInfo's own branching within this package's complexity limit.
// ok is false once the dimensions themselves cannot be read; a
// non-avc1 codec is otherwise left for isoVideoTrackAllowed's own
// codec check to refuse.
func (c *cursor) videoTrackFields(ctx context.Context, stbl span, track *isoTrack) bool {
	var ok bool
	track.width, track.height, ok = c.sampleEntryDims(stbl.off, stbl.len)
	if !ok {
		return false
	}
	if track.codec != "avc1" {
		return true
	}

	track.spsWidth, track.spsHeight, track.spsOK, track.inBandOK, ok = c.avcVideoTrackChecks(ctx, stbl, track.sampleCount)
	return ok
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
// duration fields and converts them to seconds (see
// boxTimescaleDuration): mvhd (movie-level) and mdhd (track-level)
// share the exact same layout up to those two fields, which is all
// this package ever needs from either one, whenever the raw
// timescale itself does not also matter to the caller.
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
// trackInfo's own edit-list check) reuses directly, rather than
// recovering it by multiplying boxDuration's own result back out.
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
// first sample entry's own four-byte type - the codec fourcc every
// check in this package reads, never a sender's mime type or file
// extension. ok is also false when stsd's entry_count is anything
// but 1: this package, and the in-band sample scan in avcsamples.go,
// only ever read the first entry, but FFmpeg's own mov demuxer keeps
// every entry sharing a track's codec tag and can switch to a later
// one's own codec configuration mid-stream (mov_change_extradata),
// so a second avc1 entry's own avcC - carrying its own, unbounded SPS
// - would never be checked at all. A differential security review
// found exactly this gap; see docs/decisions.md.
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

// onlyChildrenAllowed reports whether every immediate child within
// [containerOff, containerOff+containerLen) is one of allowed's own
// box types, refusing the container outright the moment one is not -
// the same "cannot read it, cannot trust it" rule children and
// firstChild already apply to a malformed box, extended here to a
// box this package simply does not expect to find there (see
// isoBMFFPlayable, trackChildBoxes and docs/decisions.md on the cmov
// gap this closes).
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
