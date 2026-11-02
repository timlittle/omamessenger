package videosafe_test

// isobmff_allowlist_test.go checks this package's own moov/trak/
// mdia/minf/stbl allowlists (see isobmff.go) against a box none of
// them name - a compressed cmov especially, the exact gap a
// differential security review found - at every level they now
// apply to: split out to keep isobmff_test.go under this codebase's
// own file-length limit, the same reason isobmff_coverart_test.go
// is its own file.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// moovWithExtraChild builds a minimal, well-formed MP4 whose moov box
// holds mvhd, videoTrak, and then extraChild appended as one more
// direct child of moov - for a test proving that a box outside this
// package's own moov allowlist refuses the whole file even when every
// track moov also carries is otherwise perfectly valid.
func moovWithExtraChild(videoTrak, extraChild []byte) []byte {
	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	moovBody := append(append(append([]byte{}, mvhd...), videoTrak...), extraChild...)
	out := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
	return append(out, mp4Box("moov", moovBody)...)
}

// TestValidate_MP4CompressedOrUnknownMoovChild covers the exact gap a
// differential security review found: FFmpeg's own mov demuxer hands
// a cmov box to mov_read_cmov, which inflates its own declared size
// and reparses the result as a second, completely unvalidated moov -
// every track inside becomes a stream this package never looked at.
// An otherwise ordinary video track alongside one must not make the
// file playable, the same for mvex (a fragmented MP4's own sample
// tables) and for a box this package simply does not name at all, to
// prove this is a real allowlist and not a hand-picked list of two
// named shapes.
func TestValidate_MP4CompressedOrUnknownMoovChild(t *testing.T) {
	t.Parallel()

	videoTrak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})

	cases := map[string][]byte{
		"compressed moov (cmov) alongside an otherwise valid moov":          moovWithExtraChild(videoTrak, mp4Box("cmov", []byte("fake compressed movie data, never inflated or trusted here"))),
		"mvex (fragmented sample tables) alongside an otherwise valid moov": moovWithExtraChild(videoTrak, mp4Box("mvex", []byte("fake fragment defaults"))),
		"an unrecognized box alongside an otherwise valid moov":             moovWithExtraChild(videoTrak, mp4Box("zzzz", []byte("not a box this package recognizes"))),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "tracks.mp4", data)
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

// FuzzValidateMP4Allowlist seeds the corpus with the crafted
// allowlist-violating shapes above - a compressed moov and an
// unknown box nested inside stbl - so go test's mutation engine
// explores around this package's own moov/trak/mdia/minf/stbl
// allowlists too, the same discipline FuzzValidateMP4 in
// isobmff_test.go already applies to every other shape this package
// checks.
func FuzzValidateMP4Allowlist(f *testing.F) {
	f.Add(moovWithExtraChild(
		mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000}),
		mp4Box("cmov", []byte("fake compressed movie data")),
	))
	f.Add(mp4FixtureWithStblExtra(mp4Box("zzzz", []byte("unexpected"))))

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp4", data)
		if _, err := videosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}

// TestValidate_MP4UnknownNestedBox covers this package's own
// allowlist at every level it now applies below moov - trak, mdia,
// minf and stbl - each proven with a box this package does not name
// there, alongside the structure it still needs to read, so an
// otherwise valid track is refused the moment any one of its own
// containers carries a box FFmpeg's own mov demuxer might still open
// while this package stayed silent about it.
func TestValidate_MP4UnknownNestedBox(t *testing.T) {
	t.Parallel()

	extra := mp4Box("zzzz", []byte("not a box this package recognizes"))

	cases := map[string][]byte{
		"unknown box as a direct child of trak": mp4FixtureWithTrakExtra(extra),
		"unknown box as a direct child of mdia": mp4FixtureWithMdiaExtra(extra),
		"unknown box as a direct child of minf": mp4FixtureWithMinfExtra(extra),
		"unknown box as a direct child of stbl": mp4FixtureWithStblExtra(extra),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "nested.mp4", data)
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
