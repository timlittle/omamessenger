package audiosafe

// id3v2.go walks a leading ID3v2 tag frame by frame, rather than
// trusting its own declared size to skip it in one jump, so a frame
// carrying an embedded picture or object can never pass this package
// unexamined: FFmpeg's own id3v2 demuxer reads an APIC (or the older,
// three-letter PIC) frame as an attached picture and a GEOB (or GEO)
// frame as a general encapsulated object, either of which Qt's
// FFmpeg plugin can still decode with QImage::fromData once the file
// plays, skipping imagesafe entirely (see docs/decisions.md). Every
// frame this walk cannot fully account for - an unrecognized frame
// id, a declared size that overruns the tag, or an extended header
// this package does not parse - refuses the whole file the same way
// a forbidden frame does: this is attacker-controlled data, never
// trusted when it cannot be read in full.

// id3v2ExtendedHeaderFlag is the ID3v2 header flags byte's own bit
// marking an extended header before the first frame (ID3v2.3 and
// ID3v2.4 only). This package does not parse one, so a tag setting it
// is refused outright rather than risk mis-locating every frame after
// it.
const id3v2ExtendedHeaderFlag = 0x40

// id3v2ForbiddenFrames names every ID3v2 frame id this package treats
// as carrying an embedded picture or other object FFmpeg exposes as
// an attachment: APIC and the three-letter ID3v2.2 equivalent PIC
// ("Attached picture"), and GEOB and its own three-letter equivalent
// GEO ("General encapsulated object"), which can carry an arbitrary
// file, image included.
var id3v2ForbiddenFrames = map[string]bool{
	"APIC": true,
	"PIC":  true,
	"GEOB": true,
	"GEO":  true,
}

// id3v2Scan reports where the first MP3 frame should begin - right
// after an ID3v2 tag's own declared size when the file opens with
// one, else 0 - and whether that tag, if present, was walked all the
// way to its own end with no forbidden frame found (see
// id3v2ForbiddenFrames). safe is true with off 0 when the file has no
// ID3v2 tag at all: there is nothing to walk, so nothing to refuse.
func (c *cursor) id3v2Scan() (off int64, safe bool) {
	var hdr [id3v2HeaderLen]byte
	if !c.readAt(hdr[:], 0) || string(hdr[0:3]) != "ID3" {
		return 0, true
	}

	version := hdr[3]
	if version != 2 && version != 3 && version != 4 {
		return 0, false
	}
	if hdr[5]&id3v2ExtendedHeaderFlag != 0 {
		return 0, false
	}

	size := synchsafeSize(hdr[6:10])
	if !c.id3v2FramesSafe(id3v2HeaderLen, id3v2HeaderLen+size, version) {
		return 0, false
	}

	return id3v2HeaderLen + size, true
}

// id3v2FramesSafe walks every frame in [start, end) - the tag's own
// body, right after its header - reporting false the moment one
// fails to parse, overruns end, or is named in id3v2ForbiddenFrames.
// A frame id read as all zero bytes marks the tag's own padding
// (common after its last real frame); the walk stops there and
// reports true, since nothing forbidden was found in anything it did
// read.
func (c *cursor) id3v2FramesSafe(start, end int64, version byte) bool {
	off := start
	for off < end {
		id, bodyLen, headerLen, ok := c.id3v2FrameHeader(off, version)
		if !ok {
			return false
		}
		if id == "" {
			return true
		}
		if id3v2ForbiddenFrames[id] {
			return false
		}

		off += headerLen + bodyLen
		if off > end {
			return false
		}
	}

	return true
}

// id3v2FrameHeader reads and validates one ID3v2 frame header at off:
// a 3-byte id and 3-byte size for ID3v2.2 (version 2), or a 4-byte id,
// 4-byte size and 2-byte flags for ID3v2.3/2.4 (version 3 or 4), whose
// size is synchsafe only for version 4. id is "" for an all-zero id,
// the tag's own padding, never a real frame; ok is false once off
// cannot be read at all, the id contains a character no real ID3v2
// frame id ever does, or the declared size is negative. headerLen
// never depends on the file's own bytes, only version. This package's
// own box-budget counter (shared with the ISO BMFF walk, which never
// runs in the same Validate call as this one) bounds how many frames
// a single crafted tag can make this walk visit.
func (c *cursor) id3v2FrameHeader(off int64, version byte) (id string, bodyLen, headerLen int64, ok bool) {
	if c.boxes <= 0 {
		return "", 0, 0, false
	}
	c.boxes--

	idLen, sizeLen := 3, 3
	if version >= 3 {
		idLen, sizeLen = 4, 4
	}
	headerLen = int64(idLen + sizeLen)
	if version >= 3 {
		headerLen += 2 // frame flags, ID3v2.3/2.4 only
	}

	buf := make([]byte, headerLen)
	if !c.readAt(buf, off) {
		return "", 0, 0, false
	}

	if allZero(buf[:idLen]) {
		return "", 0, 0, true
	}

	id = string(buf[:idLen])
	if !validID3FrameID(id) {
		return "", 0, 0, false
	}

	bodyLen, ok = id3v2FrameSize(buf[idLen:idLen+sizeLen], version)
	return id, bodyLen, headerLen, ok
}

// id3v2FrameSize decodes a frame's own declared size field: plain
// big-endian for ID3v2.2 (3 bytes) and ID3v2.3 (4 bytes), synchsafe
// for ID3v2.4 (4 bytes, RFC-mandated so its own high bits can never
// be mistaken for an MP3 frame sync pattern). ok is false for a
// negative result, which a synchsafe field can never produce but a
// plain 32-bit one, read as a signed size, in principle could once
// cast.
func id3v2FrameSize(b []byte, version byte) (int64, bool) {
	var size int64
	switch {
	case version == 4:
		size = synchsafeSize(b)
	case len(b) == 3:
		size = int64(b[0])<<16 | int64(b[1])<<8 | int64(b[2])
	default:
		size = int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
	}

	return size, size >= 0
}

// allZero reports whether every byte in b is zero.
func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// validID3FrameID reports whether id's every byte is an uppercase
// ASCII letter or digit, the only characters a real ID3v2 frame id
// ever uses.
func validID3FrameID(id string) bool {
	for i := 0; i < len(id); i++ {
		b := id[i]
		if (b < 'A' || b > 'Z') && (b < '0' || b > '9') {
			return false
		}
	}
	return true
}
