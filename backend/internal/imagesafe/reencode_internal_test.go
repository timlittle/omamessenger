// reencode_internal_test.go tests jpegIsProgressive and pngBitDepth
// directly, an internal test rather than a black-box one because
// reencodeStill only ever calls them on data that has already passed
// image.DecodeConfig: the malformed or unusual shapes their own edge
// cases guard against - a gap byte or a run of fill bytes before a
// real marker, a TEM or restart marker with no length field to skip,
// an "FF 00" byte-stuffed pair, a segment declaring a length under
// two, data with no SOF marker at all, or a PNG too short to even
// hold its own bit depth byte - can never be reached by handing
// Reencode a crafted file, since DecodeConfig would already have
// refused anything that malformed. Every case this walk cannot
// actually confirm is cheap - the last three above - answers true (or
// 16, for pngBitDepth): charging more than a shape turns out to need
// is always safe, charging less never is (see docs/decisions.md).
package imagesafe

import "testing"

// TestJPEGIsProgressive covers every branch of its own marker walk
// that a real, DecodeConfig-valid JPEG never takes: SOF always
// follows immediately after the previous marker's own declared
// length in a well-formed file, so a gap byte, a fill byte or a
// restart marker ahead of it only ever appears in a hand-built input
// like these.
func TestJPEGIsProgressive(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"gap byte before a baseline SOF": {
			data: []byte{0xFF, 0xD8, 0x00, 0xFF, 0xC0, 0x00, 0x08, 0, 0, 0, 0, 0, 0},
			want: false,
		},
		"fill byte before a progressive SOF": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xFF, 0xC2, 0x00, 0x08, 0, 0, 0, 0, 0, 0},
			want: true,
		},
		"restart marker has no length field to skip": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xD0, 0xFF, 0xC2, 0x00, 0x08, 0, 0, 0, 0, 0, 0},
			want: true,
		},
		"FF 00 before any SOF is never a marker with its own length": {
			data: []byte{0xFF, 0xD8, 0xFF, 0x00, 0x00, 0x09, 0xFF, 0xC0, 0x00, 0x08, 0, 0, 0, 0, 0, 0},
			want: true,
		},
		"a segment declaring a length under two": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01},
			want: true,
		},
		"no SOF marker anywhere in the data": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xD9},
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := jpegIsProgressive(tc.data); got != tc.want {
				t.Errorf("jpegIsProgressive(%s) = %v, want %v", name, got, tc.want)
			}
		})
	}
}

// TestPNGBitDepth_TooShortToHoldTheByteDefaultsToSixteen covers the
// one branch Reencode's own real PNGs never take: image.DecodeConfig
// already refuses anything shorter than a full IHDR chunk, which is
// well past pngIHDRBitDepthOffset. 16, not 8: a shape this read
// cannot confirm is cheap must never be charged as if it were.
func TestPNGBitDepth_TooShortToHoldTheByteDefaultsToSixteen(t *testing.T) {
	t.Parallel()

	if got := pngBitDepth([]byte("too short to hold it")); got != 16 {
		t.Errorf("pngBitDepth(short data) = %d, want 16", got)
	}
}

// TestPNGBitDepth_RequiresIHDRAsTheFirstChunk covers the other branch
// a real PNG never takes: image/png's own decoder tolerates a chunk
// spliced in before IHDR, silently ignoring it, which shifts IHDR's
// real bit depth byte past this function's own fixed offset. data's
// chunk type at pngIHDRChunkTypeOffset is deliberately "zTXt", not
// "IHDR", standing in for that shift; its byte at pngIHDRBitDepthOffset
// is set to 8, what a real 8-bit depth would read as, to confirm the
// missing "IHDR" check is what forces 16, not merely a zero value
// defaulting there.
func TestPNGBitDepth_RequiresIHDRAsTheFirstChunk(t *testing.T) {
	t.Parallel()

	data := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'} // signature
	data = append(data, 0, 0, 0, 0)                             // chunk length (value unchecked here)
	data = append(data, 'z', 'T', 'X', 't')                     // chunk type: not "IHDR"
	data = append(data, make([]byte, pngIHDRBitDepthOffset-len(data)+1)...)
	data[pngIHDRBitDepthOffset] = 8

	if got := pngBitDepth(data); got != 16 {
		t.Errorf("pngBitDepth(no IHDR at the fixed chunk-type offset) = %d, want 16", got)
	}
}
