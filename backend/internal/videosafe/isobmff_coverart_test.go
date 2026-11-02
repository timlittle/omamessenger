package videosafe_test

// isobmff_coverart_test.go checks hasCoverArtAnywhere's own recursive
// walk (see isobmff.go) against every placement a cover art atom can
// reach FFmpeg's own mov demuxer from, beyond the one, most common
// shape isobmff_test.go's own cover-art case already covers: split out
// to keep that file under this codebase's own file-length limit.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4CoverArtPlacements covers every placement a cover
// art atom can reach FFmpeg's own mov demuxer from that the earlier,
// narrower check (looking only at moov's first udta, that udta's
// first meta, and so on) missed: a second udta sibling after an empty
// first one, meta directly under moov with no udta wrapper, a bare
// meta at the file's own top level, and a malformed decoy box placed
// inside udta ahead of the real atom (see helpers_test.go and
// docs/decisions.md). Also covers an image codec in the one video
// track slot this package allows, confirming it is already refused by
// the existing codec allowlist with no code change needed.
func TestValidate_MP4CoverArtPlacements(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"cover art under moov > meta directly, no udta wrapper": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "moovMeta"}),
			false,
		},
		"cover art under a second udta, after an empty first one": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "secondUdta"}),
			false,
		},
		"cover art behind a malformed decoy box inside udta": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "malformedDecoy"}),
			false,
		},
		"cover art in a bare meta box at the file's own top level": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "topLevel"}),
			false,
		},
		"an image codec in the video track's own codec slot": {
			// Not a cover art atom at all - a track declaring itself an
			// ordinary video track but naming an image codec FFmpeg
			// would otherwise treat as an attached picture. Already
			// refused by this package's own video codec allowlist
			// (allowedISOVideoCodec, avc1 only), the same path
			// isobmff_test.go's "vp9 video with aac audio" case covers
			// for a different unsupported codec; named separately so
			// the reason this exact shape is safe is on record.
			mp4Fixture(mp4Params{videoCodec: "mjpg", width: 640, height: 480, durationUnit: 5000}),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "fixture.mp4", tc.data)
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
