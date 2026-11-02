package videosafe_test

// avccshapedsample_test.go reproduces, on its own, the one exploit a
// security review found needs both avcC's own header bytes and a
// sample's own in-band bytes together to build: a sample shaped like
// a replacement AVCDecoderConfigurationRecord (see avcsamples.go's
// sampleShapedLikeAVCDecoderConfig). It gets its own file, rather
// than sitting beside avcsamples_test.go's other in-band NAL cases or
// avccconfig_test.go's own avcC structural cases, because it is
// neither on its own: it is the one case where both halves of this
// package's avc1 checking have to agree with each other, not just
// each pass its own bounds.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4InBandAVCCShapedSample reproduces the exact
// mechanism a security review found: a track declaring a 2-byte NAL
// length prefix, carrying one sample whose leading bytes are both a
// well-formed length-prefixed NAL (type 0, which this package's own
// scan - before this fix - judged "ordinary, unjudged slice data" and
// let through) and, read as FFmpeg's own H.264 decoder reads a
// sample's raw bytes before ever applying that length prefix, a
// structurally valid AVCDecoderConfigurationRecord. FFmpeg's decoder
// takes the second reading and replaces its trusted SPS/PPS sets with
// whatever the sample declares; this package must now refuse the
// sample outright instead of judging it by the first reading alone.
func TestValidate_MP4InBandAVCCShapedSample(t *testing.T) {
	t.Parallel()

	// configurationVersion 1, AVCProfileIndication/profile_compatibility
	// 0/0 - profile_compatibility 0 doubling as this sample's own first
	// NAL's length-prefix low byte, so the 2-byte prefix reads as
	// length 0x0100 (256) - level_idc 0x32, lengthSizeMinusOne 3
	// (reserved bits all set), one declared SPS (4 arbitrary bytes) and
	// one declared PPS (2 arbitrary bytes): exactly the shape FFmpeg's
	// own is_avcc_extradata check walks successfully. The sample is
	// padded with zero bytes to 258 total: 2 (this package's own
	// declared NAL length prefix) + 256 (the NAL length those same two
	// bytes, read as 0x0100, declare), so a validator that still read
	// this sample as one length-prefixed NAL of type 0 would have
	// accepted it as ordinary slice data.
	avccShapedSample := append([]byte{
		1, 0, 0, 0x32, 0xFF, 0xE1, // configurationVersion, profile, compat, level, lengthSize byte, numSPS
		0, 4, 0x67, 0x01, 0x02, 0x03, // one 4-byte "SPS"
		1,                // numPPS
		0, 2, 0x68, 0x80, // one 2-byte "PPS"
	}, make([]byte, 258-17)...)

	data := mp4AVCVideoFixtureWithAVCC(1280, 720, buildAVCDecoderConfigWithLengthSize(1280, 720, 1), [][]byte{avccShapedSample})

	path := writeFixture(t, "avcc-shaped-sample.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a sample shaped like a replacement AVCDecoderConfigurationRecord, want it refused")
	}
}
