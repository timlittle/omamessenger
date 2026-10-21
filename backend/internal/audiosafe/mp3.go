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
// means here.

// id3v2HeaderLen is the fixed size of an ID3v2 tag header this
// package skips to find the first MP3 frame: "ID3", two version
// bytes, one flags byte, four bytes of synchsafe size.
const id3v2HeaderLen = 10

// validateMP3 reports whether c's file is a simple, constant-bitrate
// MPEG-1 Layer III stream within maxDuration: its first frame header
// (after skipping an optional leading ID3v2 tag) declares an allowed
// sample rate and a non-free, non-reserved bitrate, and
// mp3FramesToConfirm consecutive frames are found each at the offset
// the previous one's own declared length predicts. Duration is
// estimated from the file's total size and that one bitrate, which
// is only accurate for a constant-bitrate stream - exactly the case
// this package accepts.
func validateMP3(c *cursor) bool {
	off := c.mp3StartOffset()

	bitrateKbps, sampleRateHz, ok := c.confirmMP3Frames(off)
	if !ok {
		return false
	}

	durationSec := float64(c.size-off) * 8 / float64(bitrateKbps*1000)
	return durationSec > 0 && durationSec <= maxDuration.Seconds() && sampleRateHz > 0
}

// mp3StartOffset reports where the first MP3 frame should begin:
// right after an ID3v2 tag's own declared size when the file opens
// with one, else 0.
func (c *cursor) mp3StartOffset() int64 {
	var hdr [id3v2HeaderLen]byte
	if !c.readAt(hdr[:], 0) || string(hdr[0:3]) != "ID3" {
		return 0
	}

	size := synchsafeSize(hdr[6:10])
	return id3v2HeaderLen + size
}

// synchsafeSize decodes an ID3v2 synchsafe size field: four bytes,
// each holding only its low 7 bits, big-endian.
func synchsafeSize(b []byte) int64 {
	return int64(b[0])<<21 | int64(b[1])<<14 | int64(b[2])<<7 | int64(b[3])
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
