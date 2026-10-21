package videosafe_test

// matroska_test.go checks Validate against WebM/Matroska fixtures
// (see helpers_test.go's webmFixture) and fuzzes the same EBML walker
// against arbitrary bytes, with particular attention to a crafted
// huge declared element size: EBML's variable-length size field can
// claim up to 2^56-1 bytes in a single element, and this package must
// reject that as malformed rather than let the arithmetic overflow
// and accept it.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_WebM covers an ordinary, well-formed video and every
// way this package's own limits reject one, the WebM/Matroska
// equivalent of TestValidate_MP4.
func TestValidate_WebM(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"ordinary vp9 video, no audio": {
			webmFixture(webmParams{width: 1280, height: 720, durationTicks: 5000}),
			true,
		},
		"vp8 video with vorbis audio": {
			webmFixture(webmParams{videoCodec: "V_VP8", audioCodec: "A_VORBIS", width: 640, height: 480, durationTicks: 5000}),
			true,
		},
		"av1 video with opus audio": {
			webmFixture(webmParams{videoCodec: "V_AV1", audioCodec: "A_OPUS", width: 1920, height: 1080, durationTicks: 5000}),
			true,
		},
		"unsupported video codec": {
			webmFixture(webmParams{videoCodec: "V_THEORA", width: 1280, height: 720, durationTicks: 5000}),
			false,
		},
		"width over the cap": {
			webmFixture(webmParams{width: 1921, height: 1080, durationTicks: 5000}),
			false,
		},
		"height over the cap": {
			webmFixture(webmParams{width: 1080, height: 1921, durationTicks: 5000}),
			false,
		},
		"zero dimensions": {
			webmFixture(webmParams{width: 0, height: 0, durationTicks: 5000}),
			false,
		},
		"unsupported audio codec": {
			webmFixture(webmParams{audioCodec: "A_AC3", width: 1280, height: 720, durationTicks: 5000}),
			false,
		},
		"duration over the cap": {
			webmFixture(webmParams{width: 1280, height: 720, timecodeScale: 1_000_000_000, durationTicks: 11 * 60}),
			false,
		},
		"zero duration": {
			webmFixture(webmParams{width: 1280, height: 720, durationTicks: 0}),
			false,
		},
		"not a video file at all": {
			[]byte("this is just some ordinary text, not a video of any kind"),
			false,
		},
		"empty file": {
			[]byte{},
			false,
		},
		"truncated tracks element": {
			webmFixture(webmParams{width: 1280, height: 720, durationTicks: 5000})[:40],
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "fixture.webm", tc.data)
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestValidate_WebMHugeElementSize reproduces the exact overflow risk
// an EBML size vint invites: its first byte here declares an 8-byte
// size vint whose value (2^56-2, deliberately one below Matroska's
// own reserved "unknown size" sentinel, which is every data bit set)
// is already far larger than the whole file. A naive bounds check
// (offset+size compared to a limit) would wrap this negative in a
// 64-bit sum and accept it as if it fit; this package's own check
// compares size against the limit itself and must reject it instead,
// without panicking or hanging.
func TestValidate_WebMHugeElementSize(t *testing.T) {
	t.Parallel()

	data := append([]byte{0x1A, 0x45, 0xDF, 0xA3}, 0x80) // EBML header, empty body
	// Segment ID, then an 8-byte size vint declaring 2^56-2 bytes,
	// followed by only a few bytes - nowhere near enough to actually
	// hold a Segment this size.
	data = append(data, 0x18, 0x53, 0x80, 0x67)
	data = append(data, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE)
	data = append(data, 0x00, 0x00, 0x00, 0x00)

	path := writeFixture(t, "huge-size.webm", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a Segment whose declared size cannot possibly fit in the file, want false")
	}
}

// TestValidate_WebMMalformedStructure covers every element this
// package's track walker needs that can go missing, each of which
// must reject the file rather than treat a track it could not fully
// read as safe.
func TestValidate_WebMMalformedStructure(t *testing.T) {
	t.Parallel()

	segmentWith := func(body []byte) []byte {
		out := ebmlElement(testEBMLHeaderID, []byte{})
		return append(out, ebmlElement(testIDSegment, body)...)
	}

	info := ebmlElement(testIDTimecodeScale, ebmlUint(1_000_000))
	info = append(info, ebmlElement(testIDDuration, ebmlFloat64(5000))...)
	infoElem := ebmlElement(testIDInfo, info)

	videoEntryMissingVideoElem := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeVideo))
	videoEntryMissingVideoElem = append(videoEntryMissingVideoElem, ebmlElement(testIDCodecID, []byte("V_VP9"))...)

	videoEntryMissingHeight := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeVideo))
	videoEntryMissingHeight = append(videoEntryMissingHeight, ebmlElement(testIDCodecID, []byte("V_VP9"))...)
	videoEntryMissingHeight = append(videoEntryMissingHeight, ebmlElement(testIDVideo, ebmlElement(testIDPixelWidth, ebmlUint(640)))...)

	videoEntryMissingCodec := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeVideo))

	videoEntryMissingType := ebmlElement(testIDCodecID, []byte("V_VP9"))

	longCodec := ebmlElement(testIDTrackType, ebmlUint(testTrackTypeVideo))
	longCodec = append(longCodec, ebmlElement(testIDCodecID, make([]byte, 512))...)

	cases := map[string][]byte{
		"no EBML header at all":                  []byte("just some random bytes, no EBML magic"),
		"no Segment at all":                      ebmlElement(testEBMLHeaderID, []byte{}),
		"Segment body starts with an invalid ID": segmentWith([]byte{0x00, 0x00, 0x00, 0x00}),
		"Segment with no Info":                   segmentWith(ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingVideoElem))),
		"Info with no Duration": segmentWith(append(
			ebmlElement(testIDInfo, ebmlElement(testIDTimecodeScale, ebmlUint(1_000_000))),
			ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingVideoElem))...)),
		"Segment with no Tracks":          segmentWith(infoElem),
		"audio-only, no video track":      segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlAudioTrackEntry("A_AAC"))...)),
		"video TrackEntry with no Video":  segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingVideoElem))...)),
		"Video with no PixelHeight":       segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingHeight))...)),
		"TrackEntry with no CodecID":      segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingCodec))...)),
		"TrackEntry with no TrackType":    segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingType))...)),
		"CodecID far longer than allowed": segmentWith(append(append([]byte{}, infoElem...), ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, longCodec))...)),
		"Duration of an invalid byte length": segmentWith(append(
			ebmlElement(testIDInfo, append(ebmlElement(testIDTimecodeScale, ebmlUint(1_000_000)), ebmlElement(testIDDuration, []byte{1, 2})...)),
			ebmlElement(testIDTracks, ebmlElement(testIDTrackEntry, videoEntryMissingVideoElem))...)),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "malformed.webm", data)
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got {
				t.Errorf("Validate() = true for %q, want false", name)
			}
		})
	}
}

// FuzzValidateWebM seeds the corpus with the crafted cases an EBML
// parser must survive without panicking, hanging or reading past its
// own bounds: a valid file, one with an oversize dimension, one with
// an unsupported codec, one truncated partway through an element
// header, one with a huge declared element size, and one that is not
// a video at all. go test's mutation engine takes it from there.
func FuzzValidateWebM(f *testing.F) {
	f.Add(webmFixture(webmParams{width: 1280, height: 720, durationTicks: 5000}))
	f.Add(webmFixture(webmParams{width: 4000, height: 4000, durationTicks: 5000}))
	f.Add(webmFixture(webmParams{videoCodec: "V_THEORA", width: 640, height: 480, durationTicks: 5000}))
	f.Add(webmFixture(webmParams{width: 1280, height: 720, durationTicks: 5000})[:40])
	f.Add([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x80, 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE})
	f.Add([]byte("not a video file at all"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.webm", data)
		if _, err := videosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
