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
	"strings"
)

// oggPageHeaderLen is the fixed part of every Ogg page header, up to
// and including the page_segments count: capture pattern (4),
// version (1), header_type (1), granule_position (8),
// bitstream_serial_number (4), page_sequence_number (4), CRC (4),
// page_segments (1).
const oggPageHeaderLen = 27

// oggHeaderBOS is the header_type flag marking a page as the first of
// its logical bitstream. validateOgg no longer has a matching
// end-of-stream flag to check: it now reads every page to the file's
// own end rather than stopping at the first stream's own EOS page
// (see docs/decisions.md), so that flag is never tested on its own.
const oggHeaderBOS = 0x02

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
// stream within maxDuration: every page in the file, to its very
// last byte, shares one bitstream serial number (a second, distinct
// serial would mean a multiplexed or chained stream - which could
// carry video - so that rejects the whole file outright, even when
// it appears only after the first stream's own EOS page: FFmpeg's
// own Ogg demuxer still opens a chained stream that starts there, so
// this package must keep reading to the end of the file rather than
// stop at the first EOS it sees), the first page is marked BOS and
// its payload opens with opusHeadMagic, and the highest granule
// position any page reports, divided by Opus's fixed 48kHz granule
// rate, is greater than zero and no more than maxDuration.
func validateOgg(c *cursor) bool {
	st, ok := walkOggPages(c)
	if !ok || !st.sawOpusHead || st.maxGranule == 0 {
		return false
	}

	return float64(st.maxGranule)/float64(opusGranuleRate) <= maxDuration.Seconds()
}

// walkOggPages reads every page of c in order, at most maxOggPages,
// checking each one as validateOgg describes, and reports the state it
// gathered; ok is false if any page fails or the pages do not end
// exactly at the end of the file.
func walkOggPages(c *cursor) (st oggWalkState, ok bool) {
	off := int64(0)
	for pages := 0; pages < maxOggPages && off < c.size; pages++ {
		hdr, segments, headerOK := c.readOggPageHeader(off)
		if !headerOK || !st.acceptSerial(hdr) {
			return st, false
		}

		payloadOff := off + oggPageHeaderLen + int64(len(segments))
		firstPage := !st.sawOpusHead
		if firstPage && !st.confirmOpusHead(c, payloadOff) {
			return st, false
		}
		st.trackGranule(hdr.granule)

		off = payloadOff + sumSegments(segments)
		if firstPage && !c.opusCommentHeaderSafe(off, hdr.serial) {
			return st, false
		}
	}

	return st, off == c.size
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

// maxOpusCommentPacketLen bounds how many bytes of an Opus stream's
// own comment header packet ("OpusTags") this package will ever
// reassemble across Ogg page continuations before giving up: a real
// comment header carries only a vendor string and a handful of short
// "KEY=VALUE" tags, nowhere near this. A hostile file that pads past
// it, or embeds a cover image's own base64 bytes directly inside a
// METADATA_BLOCK_PICTURE tag, is refused outright rather than scanned
// in full (see docs/decisions.md).
const maxOpusCommentPacketLen = 1 << 20 // 1 MiB

// opusTagsMagic is the eight-byte packet magic that opens an Opus
// stream's second packet, its comment header (RFC 7845 section 5.2).
var opusTagsMagic = []byte("OpusTags")

// opusForbiddenCommentKeys names every Vorbis comment field (RFC 7845's
// comment header reuses the Vorbis comment format) FFmpeg's own Ogg
// demuxer reads as an attached picture rather than plain text: the
// standard METADATA_BLOCK_PICTURE (a base64-encoded FLAC PICTURE
// block) and the older, non-standard but still-recognized COVERART. A
// comment field's own name is case-insensitive (RFC 7845 section
// 5.2), so every comparison against this list is too.
var opusForbiddenCommentKeys = []string{"METADATA_BLOCK_PICTURE", "COVERART"}

// opusCommentHeaderSafe reports whether the Opus comment header packet
// starting at off - reassembled across as many Ogg page continuations
// as maxOpusCommentPacketLen allows, every page required to share
// serial, the stream's own bitstream serial - opens with opusTagsMagic
// and carries no comment field named in opusForbiddenCommentKeys.
// Anything that prevents fully confirming that - a packet this
// package cannot read within its own bound, a page that fails to
// parse at all, or one carrying a different serial - reports false,
// refusing the whole file the same way any other check in this
// package does: this is attacker-controlled data, never trusted when
// it cannot be read in full.
func (c *cursor) opusCommentHeaderSafe(off int64, serial uint32) bool {
	packet, ok := c.readOggPacket(off, serial, maxOpusCommentPacketLen)
	if !ok || len(packet) < len(opusTagsMagic) || !bytes.Equal(packet[:len(opusTagsMagic)], opusTagsMagic) {
		return false
	}

	return !opusCommentsCarryPicture(packet[len(opusTagsMagic):])
}

// readOggPacket reassembles one logical Ogg packet starting at off -
// the first byte of a page's own payload, never partway through one -
// across as many page continuations (a page whose own last segment
// table entry is 255, meaning the packet carries on into the next
// page) as it takes to reach a segment under 255, which ends it.
// Every page read along the way must share serial, this stream's own
// bitstream serial, or ok is false: a second, distinct serial reaching
// here would mean the very same multiplexed- or chained-stream shape
// validateOgg's own caller already refuses. The reassembled packet is
// capped at maxLen; a packet that would need more is refused rather
// than read in full, the same "too large to verify" refusal this
// package gives any other oversized or malformed structure.
func (c *cursor) readOggPacket(off int64, serial uint32, maxLen int) (packet []byte, ok bool) {
	for pages := 0; pages < maxOggPages; pages++ {
		hdr, segments, headerOK := c.readOggPageHeader(off)
		if !headerOK || hdr.serial != serial {
			return nil, false
		}

		payloadOff := off + oggPageHeaderLen + int64(len(segments))
		segOff := payloadOff
		for _, seg := range segments {
			if len(packet)+int(seg) > maxLen {
				return nil, false
			}

			chunk := make([]byte, seg)
			if !c.readAt(chunk, segOff) {
				return nil, false
			}
			packet = append(packet, chunk...)
			segOff += int64(seg)

			if seg < 255 {
				return packet, true
			}
		}

		off = payloadOff + sumSegments(segments)
	}

	return nil, false
}

// opusCommentsCarryPicture reports whether body - an Opus comment
// header packet's own bytes, right after its "OpusTags" magic - names
// a comment field in opusForbiddenCommentKeys anywhere in its list,
// reading only the vendor string's and each comment's own declared
// length before skipping to the next: never a comment's actual value,
// except the handful of bytes needed to compare its own field name.
// true for a packet too short or inconsistent to read in full, the
// same "cannot confirm, so refuse" answer opusCommentHeaderSafe's own
// caller already gives a packet it could not reassemble at all.
func opusCommentsCarryPicture(body []byte) bool {
	vendorLen, body, ok := takeUint32LE(body)
	if !ok || int64(vendorLen) > int64(len(body)) {
		return true
	}
	body = body[vendorLen:]

	count, body, ok := takeUint32LE(body)
	if !ok {
		return true
	}

	for i := uint32(0); i < count; i++ {
		var entryLen uint32
		entryLen, body, ok = takeUint32LE(body)
		if !ok || int64(entryLen) > int64(len(body)) {
			return true
		}

		entry := body[:entryLen]
		body = body[entryLen:]
		if commentKeyForbidden(entry) {
			return true
		}
	}

	return false
}

// takeUint32LE reads a 4-byte little-endian integer from the front of
// buf, reporting it and the bytes that follow; ok is false once buf is
// too short to hold even that one field.
func takeUint32LE(buf []byte) (value uint32, rest []byte, ok bool) {
	if len(buf) < 4 {
		return 0, nil, false
	}
	return binary.LittleEndian.Uint32(buf[:4]), buf[4:], true
}

// commentKeyForbidden reports whether entry, one "KEY=VALUE" comment
// from an Opus comment header, names a field in
// opusForbiddenCommentKeys: compared case-insensitively against the
// bytes up to entry's own "=", per RFC 7845's case-insensitive field
// names, never the value that follows it. A comment with no "=" at
// all names no field, so it can never match.
func commentKeyForbidden(entry []byte) bool {
	eq := bytes.IndexByte(entry, '=')
	if eq < 0 {
		return false
	}

	key := entry[:eq]
	for _, forbidden := range opusForbiddenCommentKeys {
		if len(key) == len(forbidden) && strings.EqualFold(string(key), forbidden) {
			return true
		}
	}

	return false
}
