// reencode_internal_test.go tests jpegIsProgressive and pngBitDepth
// directly, an internal test rather than a black-box one because
// reencodeStill only ever calls them on data that has already passed
// image.DecodeConfig: the malformed or unusual shapes their own edge
// cases guard against - a gap byte or a run of fill bytes before a
// real marker, a TEM or restart marker with no length field to skip,
// a segment declaring a length under two, data with no SOF marker at
// all, or a PNG too short to even hold its own bit depth byte - can
// never be reached by handing Reencode a crafted file, since
// DecodeConfig would already have refused anything that malformed.
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
		"a segment declaring a length under two": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01},
			want: false,
		},
		"no SOF marker anywhere in the data": {
			data: []byte{0xFF, 0xD8, 0xFF, 0xD9},
			want: false,
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

// TestPNGBitDepth_TooShortToHoldTheByteDefaultsToEight covers the one
// branch Reencode's own real PNGs never take: image.DecodeConfig
// already refuses anything shorter than a full IHDR chunk, which is
// well past pngIHDRBitDepthOffset.
func TestPNGBitDepth_TooShortToHoldTheByteDefaultsToEight(t *testing.T) {
	t.Parallel()

	if got := pngBitDepth([]byte("too short to hold it")); got != 8 {
		t.Errorf("pngBitDepth(short data) = %d, want 8", got)
	}
}
