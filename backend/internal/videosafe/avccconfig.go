package videosafe

// avccconfig.go reads and validates an avc1 sample entry's avcC box
// itself (ISO/IEC 14496-15's AVCDecoderConfigurationRecord): its fixed
// six-byte header, the one trusted SPS it must declare, and - a
// security review found this package never checked any of these -
// that configurationVersion is exactly 1, that the header's own
// profile/compatibility/level bytes agree with the SPS's own copy of
// the same three fields, that lengthSizeMinusOne is a value ISO/IEC
// 14496-15 actually defines, and that nothing follows the SPS but
// avcC's own picture-parameter-set list and, only when the SPS's own
// profile calls for it, the matching chroma/bit-depth extension. Any
// of this left unchecked lets a crafted avcC carry a second,
// unbounded SPS in bytes FFmpeg's own avcC-mode decoder still reads
// from the very same box (see docs/decisions.md). h264sps.go does the
// actual SPS bitstream parsing this file calls into; this file is only
// about the box around it.

import (
	"context"
	"encoding/binary"
)

// avcDecoderConfig is every field this package reads from an avcC
// box's own one trusted SPS: the coded width and height it decodes
// to, its own declared seq_parameter_set_id, its raw encoded NAL
// bytes (NAL header included, emulation prevention left in place, so
// it can be compared byte for byte against a sample's own copy), and
// lengthSizeMinusOne+1 - the byte width avcC declares for every
// sample's own NAL length prefixes, which avcsamples.go needs to walk
// a sample's NAL units at all.
type avcDecoderConfig struct {
	width, height int
	spsID         uint32
	sps           []byte
	lengthSize    int
}

// avcSPSDims reads trak's stbl box for its video sample entry's avcC
// configuration, and from it the single SPS the entry's own
// numOfSequenceParameterSets field must declare, returning the coded
// width and height that SPS decodes to. ok is false whenever avcC is
// missing, declares anything other than exactly one SPS - more than
// one, or zero - or that SPS cannot itself be fully parsed: an avcC
// this package cannot read all the way through is never treated as
// safe just because its container-declared dimensions looked fine.
func (c *cursor) avcSPSDims(stblOff, stblLen int64) (int, int, bool) {
	cfg, ok := c.avcConfigDetails(stblOff, stblLen)
	if !ok {
		return 0, 0, false
	}
	return cfg.width, cfg.height, true
}

// avcConfigDetails reads trak's stbl box for its video sample entry's
// avcC configuration, the same search avcSPSDims does, returning
// every field avcsamples.go's own in-band NAL scan needs in addition
// to the coded width and height: the SPS's own id, its raw bytes, and
// avcC's own declared NAL length-prefix width.
func (c *cursor) avcConfigDetails(stblOff, stblLen int64) (avcDecoderConfig, bool) {
	stsd, found := c.firstChild(stblOff, stblLen, "stsd")
	if !found || stsd.len < 8 {
		return avcDecoderConfig{}, false
	}

	_, entry, _, ok := c.boxHeader(stsd.off+8, stsd.off+stsd.len)
	if !ok || entry.len < avcVisualSampleEntryFixedFields {
		return avcDecoderConfig{}, false
	}

	avcc, found := c.firstChild(entry.off+avcVisualSampleEntryFixedFields, entry.len-avcVisualSampleEntryFixedFields, "avcC")
	if !found {
		return avcDecoderConfig{}, false
	}

	return c.readAVCDecoderConfig(avcc)
}

// avcDecoderConfigSPSDims reads avcc as an AVCDecoderConfigurationRecord
// (ISO/IEC 14496-15) far enough to find its first and only SPS: a
// fixed 6-byte header ending in numOfSequenceParameterSets, which must
// equal exactly one, then that one SPS's own 16-bit length and bytes,
// bounded by maxSPSLength before any of it is read.
func (c *cursor) avcDecoderConfigSPSDims(avcc span) (int, int, bool) {
	cfg, ok := c.readAVCDecoderConfig(avcc)
	if !ok {
		return 0, 0, false
	}
	return cfg.width, cfg.height, true
}

// readAVCDecoderConfig reads avcc as an AVCDecoderConfigurationRecord,
// the same bytes avcDecoderConfigSPSDims always read, additionally
// keeping the SPS's own id and raw bytes and avcC's own declared NAL
// length-prefix width (lengthSizeMinusOne+1) for avcsamples.go's own
// in-band scan: its own header (see readAVCCHeader), its one trusted
// SPS (see readAVCCSPS), and - once the SPS itself parses - that
// nothing follows it but avcC's own picture-parameter-set list and,
// only for a profile whose SPS itself carries one, the matching
// chroma/bit-depth extension (see avcCTrailingDataAllowed).
func (c *cursor) readAVCDecoderConfig(avcc span) (avcDecoderConfig, bool) {
	hdr, lengthSize, ok := c.readAVCCHeader(avcc)
	if !ok {
		return avcDecoderConfig{}, false
	}

	sps, ok := c.readAVCCSPS(avcc, hdr)
	if !ok {
		return avcDecoderConfig{}, false
	}

	width, height, spsID, ok := parseSPSFields(sps)
	if !ok || !c.avcCTrailingDataAllowed(avcc, 8+int64(len(sps)), uint32(hdr[1])) {
		return avcDecoderConfig{}, false
	}
	return avcDecoderConfig{width: width, height: height, spsID: spsID, sps: sps, lengthSize: lengthSize}, true
}

// readAVCCHeader reads avcc's own fixed six-byte header -
// configurationVersion, AVCProfileIndication, profile_compatibility,
// AVCLevelIndication, the NAL length-prefix width and
// numOfSequenceParameterSets - and validates every field this package
// does not read some other way: configurationVersion must be exactly
// 1, the one value FFmpeg's own decoder treats avcc as avcC mode for
// at all (any other value falls back to reading the whole box, and
// every sample, as an Annex B start-code stream instead); exactly one
// SPS must be declared; and lengthSizeMinusOne must be 0, 1 or 3 -
// ISO/IEC 14496-15 never defines value 2, a 3-byte length prefix. hdr
// is returned alongside lengthSize so readAVCCSPS can cross-check its
// own profile/level bytes against the SPS it reads next.
func (c *cursor) readAVCCHeader(avcc span) (hdr [6]byte, lengthSize int, ok bool) {
	if avcc.len < 8 {
		return hdr, 0, false
	}
	if !c.readAt(hdr[:], avcc.off) {
		return hdr, 0, false
	}
	if hdr[0] != 1 || hdr[5]&0x1f != 1 {
		return hdr, 0, false
	}

	lengthSizeMinusOne := hdr[4] & 0x03
	if lengthSizeMinusOne == 2 {
		return hdr, 0, false
	}
	return hdr, int(lengthSizeMinusOne) + 1, true
}

// readAVCCSPS reads avcc's one declared SPS right after its six-byte
// header - its own 16-bit length, bounded by maxSPSLength and avcc's
// own declared length before any of it is read - and confirms its own
// profile/compatibility/level bytes equal hdr's (see
// avcCProfileBytesMatchSPS) before handing it back: this package never
// trusts either copy over the other.
func (c *cursor) readAVCCSPS(avcc span, hdr [6]byte) ([]byte, bool) {
	var lengthField [2]byte
	if !c.readAt(lengthField[:], avcc.off+6) {
		return nil, false
	}

	spsLen := int64(binary.BigEndian.Uint16(lengthField[:]))
	if spsLen <= 0 || spsLen > maxSPSLength || 8+spsLen > avcc.len {
		return nil, false
	}

	sps := make([]byte, spsLen)
	if !c.readAt(sps, avcc.off+8) {
		return nil, false
	}
	if !avcCProfileBytesMatchSPS(hdr, sps) {
		return nil, false
	}
	return sps, true
}

// avcCProfileBytesMatchSPS reports whether avcC's own declared
// AVCProfileIndication, profile_compatibility and AVCLevelIndication
// bytes (hdr[1:4]) equal the same three bytes the SPS itself declares
// - profile_idc, the constraint-flags/reserved byte and level_idc,
// each read straight off the SPS's own emulation-prevention-stripped
// bytes right after its NAL header, byte-aligned since nothing before
// them in the bitstream is variable length. FFmpeg's own avcC-mode
// decoder trusts these three header bytes on their own, without ever
// cross-checking them against the SPS the same box also carries, so a
// crafted avcC could otherwise disagree with its own declared SPS for
// any of them - this package refuses that outright rather than trust
// either copy over the other (see docs/decisions.md).
func avcCProfileBytesMatchSPS(hdr [6]byte, sps []byte) bool {
	if len(sps) < 1 || sps[0]&0x1f != 7 {
		return false
	}

	rbsp := removeEmulationPrevention(sps[1:])
	if len(rbsp) < 3 {
		return false
	}
	return hdr[1] == rbsp[0] && hdr[2] == rbsp[1] && hdr[3] == rbsp[2]
}

// avcCTrailingDataAllowed reads avcc's own numOfPictureParameterSets
// field and that many length-prefixed PPS NAL units, right after the
// one SPS this package already read at avcc.off+afterSPS, then refuses
// outright once any byte remains past avcc's own declared length -
// unless profileIDC is one of the Annex A profiles whose SPS itself
// carries a chroma/bit-depth extension (see highProfileChromaFields)
// and what remains is exactly that same extension, consistent with
// the SPS this package already bound to 4:2:0, 8-bit, no scaling
// matrix (see avcCChromaExtensionAllowed). This package never reads a
// PPS from avcC itself - every PPS it judges comes from a sample's own
// in-band data, checked against the SPS's id (see avcsamples.go) - so
// the list is only ever walked here to find where it ends, the same
// "cannot read it, cannot trust it" rule every other check in this
// package applies to bytes it does not otherwise need. An avcC whose
// declared length ends exactly at the SPS, with not even a
// numOfPictureParameterSets byte present, is accepted without reading
// one: ISO/IEC 14496-15 makes that field mandatory, but a box with
// zero bytes left has no room left to hide anything either way, so
// requiring the byte to exist would refuse a harmlessly non-conformant
// avcC for no safety gain.
func (c *cursor) avcCTrailingDataAllowed(avcc span, afterSPS int64, profileIDC uint32) bool {
	start := avcc.off + afterSPS
	end := avcc.off + avcc.len
	if start == end {
		return true
	}

	off, ok := c.skipAVCCPPSList(start, end)
	if !ok {
		return false
	}
	if off == end {
		return true
	}
	return highProfileChromaFields(profileIDC) && c.avcCChromaExtensionAllowed(off, end)
}

// skipAVCCPPSList reads avcC's own numOfPictureParameterSets field (a
// full byte, unlike numOfSequenceParameterSets' masked 5 bits) at off,
// then that many 2-byte-length-prefixed PPS NAL units right after it,
// each bounded by end before any of it is read, and returns the
// offset right after the last one.
func (c *cursor) skipAVCCPPSList(off, end int64) (int64, bool) {
	if off >= end {
		return 0, false
	}
	var countByte [1]byte
	if !c.readAt(countByte[:], off) {
		return 0, false
	}
	off++

	for i := 0; i < int(countByte[0]); i++ {
		var ok bool
		off, ok = c.skipAVCCLengthPrefixedEntry(off, end)
		if !ok {
			return 0, false
		}
	}
	return off, true
}

// skipAVCCLengthPrefixedEntry reads one 2-byte big-endian length at
// off, bounded by end before it is read, and returns the offset right
// after that many bytes - avcC's own shape for a single SPS, PPS or
// SPS-extension entry.
func (c *cursor) skipAVCCLengthPrefixedEntry(off, end int64) (int64, bool) {
	if off+2 > end {
		return 0, false
	}
	var lengthField [2]byte
	if !c.readAt(lengthField[:], off) {
		return 0, false
	}

	n := int64(binary.BigEndian.Uint16(lengthField[:]))
	off += 2 + n
	if off > end {
		return 0, false
	}
	return off, true
}

// avcCChromaExtensionAllowed reports whether the bytes from off to end
// are exactly avcC's own high-profile chroma/bit-depth extension
// (ISO/IEC 14496-15), consistent with the SPS this package already
// bound: chroma_format 1 (4:2:0), bit_depth_luma_minus8 and
// bit_depth_chroma_minus8 both 0 (plain 8-bit), and
// numOfSequenceParameterSetExt 0 - this package has no way to judge an
// SPS extension NAL unit, so it never accepts avcC declaring any. Any
// other length, or any of those four fields reading differently, is
// refused: those are exactly the fields parseSPSFields already
// required the SPS's own copy of these same values to hold, so a
// genuine encoder's avcC can never disagree with its own SPS here.
func (c *cursor) avcCChromaExtensionAllowed(off, end int64) bool {
	if end-off != 4 {
		return false
	}

	var ext [4]byte
	if !c.readAt(ext[:], off) {
		return false
	}
	return ext[0]&0x03 == 1 && ext[1]&0x07 == 0 && ext[2]&0x07 == 0 && ext[3] == 0
}

// highProfileChromaFields reports whether profileIDC is one of the
// H.264 profiles whose SPS carries chroma_format_idc, bit depth and a
// scaling matrix flag before the fields this package actually needs
// - the fixed profile list Annex A of the H.264 specification gives
// for exactly this purpose, never inferred from anything in the file.
// Shared by h264sps.go's own SPS parsing (chromaArrayTypeOf) and this
// file's own avcC trailing-extension check (avcCTrailingDataAllowed),
// since both need to agree on exactly the same set of profiles.
func highProfileChromaFields(profileIDC uint32) bool {
	switch profileIDC {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

// avcVideoTrackChecks reads an avc1 video track's own avcC
// configuration - the SPS-derived dimensions isoVideoTrackAllowed
// already required - and, when the track has any samples at all,
// walks every one of them for an in-band parameter set FFmpeg would
// still honor even though this package never trusted it (see
// avcsamples.go and docs/decisions.md). ok is false whenever avcC
// itself cannot be fully read, the same "cannot read it, cannot trust
// it" rule every other check in this package already applies;
// inBandOK is a separate verdict, since a track can have a perfectly
// good avcC and still fail the sample scan.
func (c *cursor) avcVideoTrackChecks(ctx context.Context, stbl span, sampleCount uint32) (spsWidth, spsHeight int, spsOK, inBandOK, ok bool) {
	cfg, found := c.avcConfigDetails(stbl.off, stbl.len)
	if !found {
		return 0, 0, false, false, false
	}

	inBandOK = c.avcSamplesSafe(ctx, stbl, sampleCount, cfg)
	return cfg.width, cfg.height, true, inBandOK, true
}
