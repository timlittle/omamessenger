package audiosafe

// ogg.go walks an Ogg container's own page headers far enough to
// confirm it carries exactly one logical bitstream, that bitstream is
// Opus (identified by its first packet's own "OpusHead" magic, never
// a sender's file name or mime type), and its last page's granule
// position - Opus's own fixed-rate sample clock (RFC 7845) - implies
// a duration within maxDuration. Every page's payload is skipped by
// arithmetic on its own segment table, never read, so a file's actual
// audio data never passes through this package at all.

import (
	"bytes"
	"encoding/binary"
)

// oggPageHeaderLen is the fixed part of every Ogg page header, up to
// and including the page_segments count: capture pattern (4),
// version (1), header_type (1), granule_position (8),
// bitstream_serial_number (4), page_sequence_number (4), CRC (4),
// page_segments (1).
const oggPageHeaderLen = 27

// oggHeaderBOS is the header_type flag marking a page as the first of
// its logical bitstream.
const oggHeaderBOS = 0x02

// oggHeaderEOS is the header_type flag marking a page as the last of
// its logical bitstream.
const oggHeaderEOS = 0x04

// opusHeadMagic is the eight-byte packet magic that opens an Opus
// stream's first page (RFC 7845 section 5.1), read from the page's
// own payload, never trusted from anything the sender claims.
var opusHeadMagic = []byte("OpusHead")

// oggWalkState is validateOgg's own running state across pages: the
// one bitstream serial every page must share, the highest granule
// position seen, and whether the first page's payload was confirmed
// to open with opusHeadMagic. Carrying this as a small type rather
// than validateOgg's own local variables keeps each page's checks in
// their own short method.
type oggWalkState struct {
	serial      uint32
	haveSerial  bool
	maxGranule  uint64
	sawOpusHead bool
}

// validateOgg reports whether c's file is a single-bitstream Opus
// stream within maxDuration: every page shares one bitstream serial
// number (a second, distinct serial would mean a multiplexed or
// chained stream - which could carry video - so that rejects the
// whole file outright), the first page is marked BOS and its payload
// opens with opusHeadMagic, and the highest granule position any
// page reports, divided by Opus's fixed 48kHz granule rate, is
// greater than zero and no more than maxDuration.
func validateOgg(c *cursor) bool {
	var st oggWalkState

	off := int64(0)
	for pages := 0; pages < maxOggPages && off < c.size; pages++ {
		hdr, segments, ok := c.readOggPageHeader(off)
		if !ok || !st.acceptSerial(hdr) {
			return false
		}

		payloadOff := off + oggPageHeaderLen + int64(len(segments))
		if !st.sawOpusHead && !st.confirmOpusHead(c, payloadOff) {
			return false
		}
		st.trackGranule(hdr.granule)

		off = payloadOff + sumSegments(segments)
		if hdr.headerType&oggHeaderEOS != 0 {
			break
		}
	}

	if !st.sawOpusHead || st.maxGranule == 0 {
		return false
	}

	return float64(st.maxGranule)/float64(opusGranuleRate) <= maxDuration.Seconds()
}

// acceptSerial records hdr's bitstream serial the first time it is
// called, requiring that first page be marked BOS, and on every later
// call reports whether hdr still names that same serial: a second,
// distinct serial means a multiplexed or chained stream, which this
// package never trusts (see validateOgg's own doc comment).
func (s *oggWalkState) acceptSerial(hdr oggPageHeader) bool {
	if !s.haveSerial {
		if hdr.headerType&oggHeaderBOS == 0 {
			return false
		}
		s.serial = hdr.serial
		s.haveSerial = true
		return true
	}

	return hdr.serial == s.serial
}

// confirmOpusHead reads the eight bytes at payloadOff and records
// whether they are opusHeadMagic, reporting the same answer: this is
// only ever called for the stream's first page, whose payload must
// open with that packet for the file to be Opus at all.
func (s *oggWalkState) confirmOpusHead(c *cursor, payloadOff int64) bool {
	var magic [8]byte
	if !c.readAt(magic[:], payloadOff) {
		return false
	}

	s.sawOpusHead = bytes.Equal(magic[:], opusHeadMagic)
	return s.sawOpusHead
}

// trackGranule raises maxGranule to granule when it is higher, unless
// granule is Ogg's own "unknown" sentinel (every bit set), which many
// pages legitimately carry and must never be read as a real position.
func (s *oggWalkState) trackGranule(granule uint64) {
	if granule != 0xFFFFFFFFFFFFFFFF && granule > s.maxGranule {
		s.maxGranule = granule
	}
}

// oggPageHeader is the handful of an Ogg page header's own fields
// this package reads.
type oggPageHeader struct {
	headerType byte
	granule    uint64
	serial     uint32
}

// readOggPageHeader reads one page header at off, reporting its
// fields and segment table (up to 255 single-byte entries, the
// format's own fixed maximum), or ok false once the capture pattern
// does not match or the header is too short to read - either way, a
// file that is not well-formed Ogg, never trusted further.
func (c *cursor) readOggPageHeader(off int64) (hdr oggPageHeader, segments []byte, ok bool) {
	var buf [oggPageHeaderLen]byte
	if !c.readAt(buf[:], off) {
		return oggPageHeader{}, nil, false
	}
	if string(buf[0:4]) != "OggS" {
		return oggPageHeader{}, nil, false
	}

	hdr.headerType = buf[5]
	hdr.granule = binary.LittleEndian.Uint64(buf[6:14])
	hdr.serial = binary.LittleEndian.Uint32(buf[14:18])
	segmentCount := int(buf[26])

	segments = make([]byte, segmentCount)
	if segmentCount > 0 && !c.readAt(segments, off+oggPageHeaderLen) {
		return oggPageHeader{}, nil, false
	}

	return hdr, segments, true
}

// sumSegments adds a page's segment table entries, each 0-255,
// bounded at 255 entries by the format itself, so the result can
// never overflow or require anything but simple arithmetic.
func sumSegments(segments []byte) int64 {
	var total int64
	for _, s := range segments {
		total += int64(s)
	}
	return total
}
