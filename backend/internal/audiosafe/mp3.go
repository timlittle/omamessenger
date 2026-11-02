package audiosafe

// mp3.go validates the one MP3 shape this package calls simple
// enough to trust: a constant-bitrate MPEG-1 Layer III stream, found
// by a frame header's sync pattern and confirmed by finding
// mp3FramesToConfirm further headers each exactly where the previous
// frame's own declared length says the next one starts. Anything
// else an MP3 file can legally be - MPEG-2/2.5, Layer I/II, free or
// variable bitrate - is refused outright rather than guessed at (see
// package doc and docs/decisions.md): this package never walks a
// variable-length frame chain, which is what "simple to validate"
// means here. A file is also refused outright the moment it carries
// an ID3v2, APE or Lyrics3 tag anywhere at all - not walked frame by
// frame and allowed through once no forbidden frame is found, the way
// an earlier fix did this, since that walk and FFmpeg's own id3v2
// demuxer were found to disagree about a v2.4 frame's own declared
// size (see mp3HasForbiddenTag and docs/decisions.md). A real MP3
// from an encoder almost always carries an ID3v2 tag, so this is a
// deliberately narrow in-window allowance: a refused file still opens
// fine in the user's own player.

import "bytes"

// validateMP3 reports whether c's file is a simple, constant-bitrate
// MPEG-1 Layer III stream within maxDuration, carrying none of the
// tag kinds mp3HasForbiddenTag refuses: its first frame header
// declares an allowed sample rate and a non-free, non-reserved
// bitrate, and mp3FramesToConfirm consecutive frames are found each
// at the offset the previous one's own declared length predicts. An
// ID3v1 trailer, which has no room for a picture at all (a fixed 128
// bytes of plain text fields), is left unchecked: nothing this
// package refuses matches its own "TAG" signature, so one at the very
// end - the only place the format allows - never affects this result.
// Duration is estimated from the file's total size and that one
// bitrate, which is only accurate for a constant-bitrate stream -
// exactly the case this package accepts.
func validateMP3(c *cursor) bool {
	if c.mp3HasForbiddenTag() {
		return false
	}

	bitrateKbps, sampleRateHz, ok := c.confirmMP3Frames(0)
	if !ok {
		return false
	}

	durationSec := float64(c.size) * 8 / float64(bitrateKbps*1000)
	return durationSec > 0 && durationSec <= maxDuration.Seconds() && sampleRateHz > 0
}

// mp3HasForbiddenTag reports whether c's file contains any of
// mp3ForbiddenTagMarkers anywhere at all, read in bounded, overlapping
// chunks (mp3ScanChunkSize) so a marker straddling a chunk boundary is
// never missed and the whole file is never loaded into memory at
// once. An unreadable chunk is never trusted either way: this package
// reads files it can fully account for, or refuses them, never a
// middle ground.
func (c *cursor) mp3HasForbiddenTag() bool {
	overlap := int64(mp3MaxForbiddenMarkerLen - 1)
	stride := int64(mp3ScanChunkSize) - overlap

	for off := int64(0); off < c.size; off += stride {
		end := min(off+mp3ScanChunkSize, c.size)

		buf := make([]byte, end-off)
		if !c.readAt(buf, off) {
			return true
		}
		if containsForbiddenMP3Marker(buf) {
			return true
		}
		if end == c.size {
			break
		}
	}

	return false
}

// containsForbiddenMP3Marker reports whether buf contains any byte
// sequence named in mp3ForbiddenTagMarkers.
func containsForbiddenMP3Marker(buf []byte) bool {
	for _, marker := range mp3ForbiddenTagMarkers {
		if bytes.Contains(buf, marker) {
			return true
		}
	}
	return false
}

// confirmMP3Frames reads the frame header at off and mp3FramesToConfirm-1
// further headers, each at the offset the previous frame's own
// bitrate and sample rate predict, reporting the shared bitrate and
// sample rate once every one of them parses as the same simple
// MPEG-1 Layer III shape; ok is false the moment any header fails to
// parse, is not MPEG-1 Layer III, or names a free, reserved or
// otherwise unlisted bitrate or sample rate.
func (c *cursor) confirmMP3Frames(off int64) (bitrateKbps, sampleRateHz int, ok bool) {
	for i := 0; i < mp3FramesToConfirm; i++ {
		kbps, hz, frameLen, ok := c.mp3FrameHeader(off)
		if !ok {
			return 0, 0, false
		}
		if i == 0 {
			bitrateKbps, sampleRateHz = kbps, hz
		} else if kbps != bitrateKbps || hz != sampleRateHz {
			return 0, 0, false
		}

		off += frameLen
	}

	return bitrateKbps, sampleRateHz, true
}

// mp3FrameHeader reads and validates one 4-byte MPEG-1 Layer III
// frame header at off: 11 sync bits, the version (must be MPEG-1),
// the layer (must be Layer III), a bitrate index with a listed
// bitrate, and a sample-rate index with a listed rate. frameLen is
// the frame's own declared total length in bytes (header included),
// computed from its bitrate, sample rate and padding bit, the offset
// the next frame's header must start at for a constant-bitrate
// stream.
func (c *cursor) mp3FrameHeader(off int64) (bitrateKbps, sampleRateHz int, frameLen int64, ok bool) {
	var buf [4]byte
	if !c.readAt(buf[:], off) {
		return 0, 0, 0, false
	}

	if buf[0] != 0xFF || buf[1]&0xE0 != 0xE0 {
		return 0, 0, 0, false
	}

	version := (buf[1] >> 3) & 0x03
	layer := (buf[1] >> 1) & 0x03
	if version != 0x03 || layer != 0x01 {
		return 0, 0, 0, false // not MPEG-1 Layer III
	}

	bitrateKbps, ok = mp3BitrateKbps[(buf[2]>>4)&0x0F]
	if !ok {
		return 0, 0, 0, false
	}
	sampleRateHz, ok = mp3SampleRateHz[(buf[2]>>2)&0x03]
	if !ok {
		return 0, 0, 0, false
	}

	padding := int64((buf[2] >> 1) & 0x01)
	frameLen = 144*int64(bitrateKbps)*1000/int64(sampleRateHz) + padding
	if frameLen < 4 {
		return 0, 0, 0, false
	}

	return bitrateKbps, sampleRateHz, frameLen, true
}
