package videosafe

// matroska.go walks a WebM/Matroska file's EBML element tree far
// enough to answer Validate's question and no further: the Segment's
// Info (for TimecodeScale and Duration) and Tracks (for each
// TrackEntry's type, codec and, for video, pixel dimensions). Every
// element body a caller does not ask for is skipped by arithmetic on
// its declared size, never read.

import (
	"encoding/binary"
	"math"
	"strings"
)

// EBML element IDs this package reads, as their raw, marker-bit-
// included byte value: an element's ID is always compared this way,
// never stripped the way its size is (see readVint).
const (
	idSegment       uint32 = 0x18538067
	idInfo          uint32 = 0x1549A966
	idTimecodeScale uint32 = 0x2AD7B1
	idDuration      uint32 = 0x4489
	idTracks        uint32 = 0x1654AE6B
	idTrackEntry    uint32 = 0xAE
	idTrackType     uint32 = 0x83
	idCodecID       uint32 = 0x86
	idVideo         uint32 = 0xE0
	idPixelWidth    uint32 = 0xB0
	idPixelHeight   uint32 = 0xBA
)

// trackTypeVideo and trackTypeAudio are Matroska's own TrackType
// values (the Matroska specification's fixed enumeration, not
// anything this package invents).
const (
	trackTypeVideo = 1
	trackTypeAudio = 2
)

// defaultTimecodeScale is Matroska's own default, in nanoseconds per
// tick, when a file's Info element declares none.
const defaultTimecodeScale = 1_000_000

// matroskaPlayable reports whether c's file is a playable WebM or
// Matroska file under this package's limits: one Segment holding an
// Info element with a duration within maxDuration, and a Tracks
// element with at least one video TrackEntry, each within
// maxVideoTracks using an allowed codec and within the dimension
// limits, and any audio TrackEntry using an allowed codec.
func matroskaPlayable(c *cursor) bool {
	seg, ok := c.firstElement(0, c.size, idSegment)
	if !ok {
		return false
	}

	if !c.withinDuration(seg) {
		return false
	}

	tracks, ok := c.firstElement(seg.off, seg.off+seg.len, idTracks)
	if !ok {
		return false
	}

	videoTracks := 0
	sawValidVideo := false
	for _, entry := range c.elements(tracks.off, tracks.off+tracks.len, idTrackEntry) {
		kind, codec, width, height, ok := c.trackEntryInfo(entry)
		if !ok {
			return false
		}

		isVideo, allowed := matroskaTrackAllowed(kind, codec, width, height, videoTracks)
		if !allowed {
			return false
		}
		if isVideo {
			videoTracks++
			sawValidVideo = true
		}
	}

	return sawValidVideo
}

// matroskaTrackAllowed judges one TrackEntry already read by
// trackEntryInfo: isVideo reports whether kind is trackTypeVideo (so
// the caller counts it toward maxVideoTracks), and ok reports whether
// it passes every limit that applies to its own kind - an allowed
// codec and, for a video track, its dimensions - or allows it through
// unjudged for any kind that is neither.
func matroskaTrackAllowed(kind int, codec string, width, height, videoTracks int) (isVideo, ok bool) {
	switch kind {
	case trackTypeVideo:
		if videoTracks >= maxVideoTracks || !allowedMatroskaVideoCodec[codec] {
			return true, false
		}
		return true, width > 0 && height > 0 && width <= maxDimension && height <= maxDimension
	case trackTypeAudio:
		return false, allowedMatroskaAudioCodec[codec]
	default:
		return false, true
	}
}

// withinDuration reports whether seg's Info element declares a
// duration greater than zero and no more than maxDuration, combining
// its own Duration (in ticks) with its TimecodeScale (nanoseconds per
// tick, defaulting to defaultTimecodeScale when absent) as floating
// point seconds throughout, so an absurd declared value can never
// overflow an integer conversion on the way to being rejected.
func (c *cursor) withinDuration(seg span) bool {
	info, ok := c.firstElement(seg.off, seg.off+seg.len, idInfo)
	if !ok {
		return false
	}

	scale := float64(defaultTimecodeScale)
	if ticks, ok := c.uintElement(info, idTimecodeScale); ok && ticks > 0 {
		scale = float64(ticks)
	}

	ticks, ok := c.floatElement(info, idDuration)
	if !ok {
		return false
	}

	seconds := ticks * scale / 1e9
	return seconds > 0 && seconds <= maxDuration.Seconds()
}

// trackEntryInfo reads one TrackEntry element's own TrackType and
// CodecID, and, for a video track, its Video sub-element's
// PixelWidth and PixelHeight. ok is false once any of those this
// track's own kind needs is missing, so a track whose header was cut
// off is never silently treated as safe.
func (c *cursor) trackEntryInfo(entry span) (kind int, codec string, width, height int, ok bool) {
	t, found := c.uintElement(entry, idTrackType)
	if !found {
		return
	}
	kind = int(t)

	codec, ok = c.stringElement(entry, idCodecID)
	if !ok {
		return
	}

	if kind != trackTypeVideo {
		return
	}

	video, found := c.firstElement(entry.off, entry.off+entry.len, idVideo)
	if !found {
		ok = false
		return
	}

	w, found := c.uintElement(video, idPixelWidth)
	if !found {
		ok = false
		return
	}
	h, found := c.uintElement(video, idPixelHeight)
	if !found {
		ok = false
		return
	}

	return kind, codec, int(w), int(h), true
}

// uintElement finds want as a direct child of container and reads it
// as an EBML unsigned integer (big-endian, 1-8 bytes).
func (c *cursor) uintElement(container span, want uint32) (uint64, bool) {
	body, found := c.firstElement(container.off, container.off+container.len, want)
	if !found || body.len == 0 || body.len > 8 {
		return 0, false
	}

	buf := make([]byte, body.len)
	if !c.readAt(buf, body.off) {
		return 0, false
	}

	var v uint64
	for _, b := range buf {
		v = v<<8 | uint64(b)
	}
	return v, true
}

// stringElement finds want as a direct child of container and reads
// it as an EBML ASCII string, bounded by maxStringElement before
// anything is allocated: used only for CodecID, always a handful of
// characters, so a far larger declared size is already malformed.
func (c *cursor) stringElement(container span, want uint32) (string, bool) {
	body, found := c.firstElement(container.off, container.off+container.len, want)
	if !found || body.len <= 0 || body.len > maxStringElement {
		return "", false
	}

	buf := make([]byte, body.len)
	if !c.readAt(buf, body.off) {
		return "", false
	}

	return strings.TrimRight(string(buf), "\x00"), true
}

// floatElement finds want as a direct child of container and reads
// it as an EBML float, 4 or 8 bytes, the only two widths the format
// allows.
func (c *cursor) floatElement(container span, want uint32) (float64, bool) {
	body, found := c.firstElement(container.off, container.off+container.len, want)
	if !found {
		return 0, false
	}

	switch body.len {
	case 4:
		var buf [4]byte
		if !c.readAt(buf[:], body.off) {
			return 0, false
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(buf[:]))), true
	case 8:
		var buf [8]byte
		if !c.readAt(buf[:], body.off) {
			return 0, false
		}
		return math.Float64frombits(binary.BigEndian.Uint64(buf[:])), true
	default:
		return 0, false
	}
}

// element reads the EBML element starting at off, within the
// container bounded by limit, returning its ID, body and the offset
// right after it. ok is false once the element is malformed, extends
// past limit, or this parse has already visited too many elements
// (see maxBoxBudget).
func (c *cursor) element(off, limit int64) (id uint32, body span, next int64, ok bool) {
	if c.boxes <= 0 {
		return 0, span{}, 0, false
	}
	c.boxes--

	var head [12]byte
	n := min(int64(len(head)), limit-off)
	if n <= 0 || !c.readAt(head[:n], off) {
		return 0, span{}, 0, false
	}

	id, idLen, ok := readID(head[:n])
	if !ok || int64(idLen) >= n {
		return 0, span{}, 0, false
	}

	value, sizeLen, unknown, ok := readVint(head[idLen:n])
	if !ok {
		return 0, span{}, 0, false
	}

	bodyOff := off + int64(idLen) + int64(sizeLen)
	bodyLen := int64(value)
	if unknown {
		bodyLen = limit - bodyOff
	}

	// Comparing bodyLen against the small, bounded limit-bodyOff
	// rather than bodyOff+bodyLen avoids a signed overflow: a crafted
	// huge size vint would otherwise wrap the sum negative and slip
	// past a naive check (the exact "huge EBML size" case this
	// package must never trust).
	if bodyLen < 0 || bodyLen > limit-bodyOff {
		return 0, span{}, 0, false
	}

	return id, span{off: bodyOff, len: bodyLen}, bodyOff + bodyLen, true
}

// firstElement returns the first immediate child of the given ID
// within [containerOff, containerOff+containerLen), or ok false when
// there is none or the container itself is malformed.
func (c *cursor) firstElement(containerOff, containerLen int64, want uint32) (span, bool) {
	limit := containerOff + containerLen
	for off := containerOff; off < limit; {
		id, body, next, ok := c.element(off, limit)
		if !ok {
			return span{}, false
		}
		if id == want {
			return body, true
		}
		off = next
	}
	return span{}, false
}

// elements returns every immediate child of the given ID within
// [containerOff, containerOff+containerLen); it stops, returning
// whatever it already found, at the first malformed element, the
// same way a truncated file naturally yields fewer tracks than it
// claims.
func (c *cursor) elements(containerOff, containerLen int64, want uint32) []span {
	limit := containerOff + containerLen
	var out []span
	for off := containerOff; off < limit; {
		id, body, next, ok := c.element(off, limit)
		if !ok {
			return out
		}
		if id == want {
			out = append(out, body)
		}
		off = next
	}
	return out
}

// vintLength returns the byte length an EBML variable-length integer
// starting with b0 declares: one more than its leading zero bits, or
// ok false for a leading byte of 0x00, which no valid EBML ID or size
// ever starts with.
func vintLength(b0 byte) (int, bool) {
	for i := range 8 {
		if b0&(0x80>>i) != 0 {
			return i + 1, true
		}
	}
	return 0, false
}

// readID reads an EBML element ID from the start of buf: the raw
// bytes, length-marker bits included, as a plain big-endian integer -
// this format's own convention for an ID's canonical value, unlike a
// size, whose marker bits readVint strips. Every ID this package
// compares against fits in four bytes.
func readID(buf []byte) (id uint32, length int, ok bool) {
	if len(buf) == 0 {
		return 0, 0, false
	}

	length, ok = vintLength(buf[0])
	if !ok || length > len(buf) || length > 4 {
		return 0, 0, false
	}

	var v uint32
	for i := range length {
		v = v<<8 | uint32(buf[i])
	}
	return v, length, true
}

// readVint reads an EBML variable-length integer from the start of
// buf, stripping its length-marker bits to return the plain numeric
// value: this format's own convention for a size (unlike an ID, read
// through readID instead). unknown reports the reserved all-ones
// encoding, Matroska's own way of saying "to the end of its parent".
func readVint(buf []byte) (value uint64, length int, unknown bool, ok bool) {
	if len(buf) == 0 {
		return 0, 0, false, false
	}

	length, ok = vintLength(buf[0])
	if !ok || length > len(buf) || length > 8 {
		return 0, 0, false, false
	}

	marker := byte(0x80 >> (length - 1))
	value = uint64(buf[0] &^ marker)
	for i := 1; i < length; i++ {
		value = value<<8 | uint64(buf[i])
	}

	maxValue := uint64(1)<<(7*length) - 1
	return value, length, value == maxValue, true
}
