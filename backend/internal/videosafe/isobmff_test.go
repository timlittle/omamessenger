package videosafe_test

// isobmff_test.go checks Validate against MP4/MOV fixtures (see
// helpers_test.go's mp4Fixture) and fuzzes the same box walker
// against arbitrary bytes, so a crafted or merely truncated file can
// never make it panic, hang or report a verdict this package did not
// actually check for.

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// writeFixture writes data to a file in t.TempDir() and returns its
// path.
func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestValidate_MP4 covers an ordinary, well-formed video and every
// way this package's own limits reject one: an unsupported codec, a
// dimension over the cap, too many video tracks, an absurd sample
// count, unsupported audio, too long a duration, and a box truncated
// partway through the header this package needs to read.
func TestValidate_MP4(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"ordinary h264 video, no audio": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000}),
			true,
		},
		"vp9 video with aac audio": {
			// vp09's own configuration box (vpcC) never carries coded
			// dimensions - only the compressed bitstream does - so this
			// package can never bind what FFmpeg would actually decode
			// at and refuses the codec outright.
			mp4Fixture(mp4Params{videoCodec: "vp09", audioCodec: "mp4a", width: 640, height: 480, durationUnit: 5000}),
			false,
		},
		"av1 video with opus audio": {
			// Same reasoning as vp09: av01's own configuration carries
			// no bound-able dimensions either.
			mp4Fixture(mp4Params{videoCodec: "av01", audioCodec: "Opus", width: 1920, height: 1080, durationUnit: 5000}),
			false,
		},
		"avc3 video, in-band parameter sets": {
			// avc3 may carry (or switch) its own SPS in-band, inside
			// sample data this package never reads, so it is refused
			// even though its sample entry otherwise looks like avc1's.
			mp4Fixture(mp4Params{videoCodec: "avc3", width: 1280, height: 720, durationUnit: 5000}),
			false,
		},
		"unsupported video codec": {
			mp4Fixture(mp4Params{videoCodec: "hev1", width: 1280, height: 720, durationUnit: 5000}),
			false,
		},
		"width over the cap": {
			mp4Fixture(mp4Params{width: 1921, height: 1080, durationUnit: 5000}),
			false,
		},
		"height over the cap": {
			mp4Fixture(mp4Params{width: 1080, height: 1921, durationUnit: 5000}),
			false,
		},
		"zero dimensions": {
			mp4Fixture(mp4Params{width: 0, height: 0, durationUnit: 5000}),
			false,
		},
		"absurd sample count": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, sampleCount: 50_000_000}),
			false,
		},
		"unsupported audio codec": {
			mp4Fixture(mp4Params{audioCodec: "ac-3", width: 1280, height: 720, durationUnit: 5000}),
			false,
		},
		"duration over the cap": {
			mp4Fixture(mp4Params{width: 1280, height: 720, timescale: 1, durationUnit: 11 * 60}),
			false,
		},
		"zero duration": {
			mp4Fixture(mp4Params{width: 1280, height: 720, timescale: 1000, durationUnit: 0}),
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
		"truncated moov box": {
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000})[:40],
			false,
		},
		"otherwise ordinary video with an iTunes-style cover art atom": {
			// The exact gap a differential security review found: an
			// otherwise perfectly valid video track says nothing about
			// whether moov also carries a udta/meta/ilst/covr atom, which
			// FFmpeg's own mov demuxer reads as a second, attached-picture
			// stream and Qt decodes with QImage::fromData, bypassing
			// imagesafe entirely.
			mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArt: true}),
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

// TestValidate_MP4TooManyVideoTracks covers a file whose moov box
// declares more video tracks than this package allows.
func TestValidate_MP4TooManyVideoTracks(t *testing.T) {
	t.Parallel()

	// mp4Fixture always writes exactly one video trak, so a file
	// declaring more than maxVideoTracks is built directly instead,
	// repeating the same trak box mp4Fixture itself would build.
	path := writeFixture(t, "many-tracks.mp4", mp4ManyVideoTracks(4))
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a file declaring more video tracks than allowed, want false")
	}
}

// TestValidate_MP4SecondStsdEntry covers the exact gap a differential
// security review found in this package's own stsd reading: a second
// sample entry, even one that on its own looks just as valid as the
// first, must refuse the whole track. Nothing here read past the
// first entry, but FFmpeg's own mov demuxer keeps every entry sharing
// a track's codec tag and can switch to a later one's own codec
// configuration mid-stream, so a second avc1 entry's own avcC - never
// checked by this package at all - could carry an SPS declaring
// dimensions far past this package's own cap.
func TestValidate_MP4SecondStsdEntry(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		entryCount uint32
		want       bool
	}{
		"one entry, the ordinary case":      {1, true},
		"two entries, both otherwise valid": {2, false},
		"three entries":                     {3, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "stsd-entries.mp4", mp4AVCFixtureWithStsdEntries(1280, 720, tc.entryCount))
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

// TestValidate_MP4SPSDimensions covers the exact gap the differential
// security review found: a container whose sample entry declares one
// set of dimensions while its avcC configuration's own SPS - what
// FFmpeg actually decodes at - declares another. Validate must reject
// the mismatch rather than trust the container field, reject more
// than one declared SPS (whose choice between them this package can
// never bound, since it never reads sample data), and reject a
// declared count of zero.
func TestValidate_MP4SPSDimensions(t *testing.T) {
	t.Parallel()

	matchingSPS := buildAVCDecoderConfig(1280, 720)
	hugeSPS := buildAVCDecoderConfig(16000, 16000)

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"container and SPS dimensions agree": {
			buildCustomAVCFixture(1280, 720, matchingSPS),
			true,
		},
		"small container, SPS declares dimensions far beyond the cap": {
			buildCustomAVCFixture(320, 240, hugeSPS),
			false,
		},
		"declares two sequence parameter sets": {
			buildCustomAVCFixture(1280, 720, duplicateSPSAVCConfig(1280, 720)),
			false,
		},
		"declares zero sequence parameter sets": {
			buildCustomAVCFixture(1280, 720, []byte{1, 66, 0, 30, 0xFF, 0x00}),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "sps.mp4", tc.data)
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

// avcC's own structural edge cases - configurationVersion, the
// profile/level cross-check, the reserved lengthSizeMinusOne value and
// trailing data past its picture-parameter-set list - are covered
// directly in avccconfig_test.go, the file that mirrors avccconfig.go.

// TestValidate_MP4MissingFile covers the one case Validate reports as
// an actual error: a file that cannot be opened at all.
func TestValidate_MP4MissingFile(t *testing.T) {
	t.Parallel()

	_, err := videosafe.Validate(filepath.Join(t.TempDir(), "missing.mp4"))
	if err == nil {
		t.Fatal("Validate() on a missing file succeeded")
	}
}

// TestValidate_MP4Version1MovieHeader covers mvhd's less common
// 64-bit layout (version 1), used by a file whose movie is long
// enough that a 32-bit duration would not hold it.
func TestValidate_MP4Version1MovieHeader(t *testing.T) {
	t.Parallel()

	mvhd := mp4Box("mvhd", mp4MediaHeaderV1(1000, 5000))
	trak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})
	moov := mp4Box("moov", append(append([]byte{}, mvhd...), trak...))

	path := writeFixture(t, "v1.mp4", moov)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if !got {
		t.Error("Validate() = false for a valid file whose mvhd uses the version 1 layout, want true")
	}
}

// TestValidate_MP4MalformedStructure covers every box this package's
// track walker needs that can go missing or be too short to read,
// each of which must reject the file rather than treat a track it
// could not fully read as safe.
func TestValidate_MP4MalformedStructure(t *testing.T) {
	t.Parallel()

	validMoovWithTrak := func(trak []byte) []byte {
		mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
		return mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	}

	cases := map[string][]byte{
		"no moov box at all": mp4Box("free", []byte("nothing here")),
		"trak with no mdia":  validMoovWithTrak(mp4Box("trak", []byte("not a mdia box"))),
		"mdia with no hdlr": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("minf", mp4Box("stbl", []byte{}))...)))),
		"hdlr too short": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", []byte{1, 2, 3})...),
				mp4Box("minf", mp4Box("stbl", []byte{}))...)))),
		"mdia with no mdhd": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...)), mp4Box("minf", mp4Box("stbl", []byte{}))...)))),
		"mdia with no minf": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))...)))),
		"minf with no stbl": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))...),
				mp4Box("minf", mp4Box("vmhd", []byte{}))...)))),
		"stbl with no stsd": validMoovWithTrak(mp4Box("trak", mp4Box("mdia",
			append(append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))...),
				mp4Box("minf", mp4Box("stbl", mp4Box("stsz", mp4SampleSize(0))))...)))),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "malformed.mp4", data)
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

// FuzzValidateMP4 seeds the corpus with the crafted cases a header
// parser must survive without panicking, hanging or reading past its
// own bounds: a valid file, one with an oversize dimension, one with
// an unsupported codec, one truncated partway through a box header,
// one that is not a video at all, and - the exact shapes a security
// review found readAVCDecoderConfig needed to bound beyond its SPS
// count and length-prefix width alone - a configurationVersion other
// than 1, and an otherwise valid avcC carrying unparsed bytes past its
// own picture-parameter-set list. go test's mutation engine takes it
// from there.
func FuzzValidateMP4(f *testing.F) {
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{width: 4000, height: 4000, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{videoCodec: "xxxx", width: 640, height: 480, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000})[:40])
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArt: true}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "moovMeta"}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "secondUdta"}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "malformedDecoy"}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000, coverArtPlacement: "topLevel"}))
	f.Add([]byte("not a video file at all"))
	f.Add([]byte{})
	f.Add(func() []byte {
		cfg := append([]byte{}, buildAVCDecoderConfig(1280, 720)...)
		cfg[0] = 0 // configurationVersion other than 1
		return buildCustomAVCFixture(1280, 720, cfg)
	}())
	f.Add(buildCustomAVCFixture(1280, 720, append(buildAVCDecoderConfig(1280, 720), 0x11, 0x22, 0x33, 0x44)))

	// A security review found neither an audio track's own sample
	// count nor any track's stts/ctts/stss/sgpd/sbgp/elst tables, nor
	// an avc1 or mp4a sample entry's own children, were ever checked
	// against each other at all (see sampletables.go, sampleentry.go
	// and docs/decisions.md); these seeds cover the shapes that
	// closed it.
	for _, seed := range sampleTableAndEntryFuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp4", data)
		if _, err := videosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}

// TestValidate_MP4BoxHeaderVariants covers boxHeader's own less
// common encodings and failure paths directly, each still reached
// through the public Validate function: a box using the 64-bit
// extended size (size field 1), a box extending to the end of its
// container (size field 0), a box too short to even hold a header,
// and a box whose declared size cannot fit in its container.
func TestValidate_MP4BoxHeaderVariants(t *testing.T) {
	t.Parallel()

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	trak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})
	moovBody := append(append([]byte{}, mvhd...), trak...)

	extendedSizeMoov := make([]byte, 16+len(moovBody))
	binary.BigEndian.PutUint32(extendedSizeMoov[0:4], 1)
	copy(extendedSizeMoov[4:8], "moov")
	binary.BigEndian.PutUint64(extendedSizeMoov[8:16], uint64(len(extendedSizeMoov)))
	copy(extendedSizeMoov[16:], moovBody)

	zeroSizeMoov := make([]byte, 8+len(moovBody))
	copy(zeroSizeMoov[4:8], "moov") // size left 0: "extends to end of file"
	copy(zeroSizeMoov[8:], moovBody)

	cases := map[string][]byte{
		"64-bit extended box size":          extendedSizeMoov,
		"box size 0 extends to end of file": zeroSizeMoov,
		"box too short for even a header":   {0, 0, 0},
		"box declares a size larger than its container": func() []byte {
			oversized := mp4Box("moov", moovBody)
			binary.BigEndian.PutUint32(oversized[0:4], uint32(len(oversized)+1000))
			return oversized
		}(),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "boxvariant.mp4", data)
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if name == "box declares a size larger than its container" || name == "box too short for even a header" {
				if got {
					t.Errorf("Validate() = true for %q, want false", name)
				}
				return
			}
			if !got {
				t.Errorf("Validate() = false for %q, want true", name)
			}
		})
	}
}

// TestValidate_MP4AudioStz2Refused moved to sampletables_test.go,
// once this file's own file-length limit needed the room: it covers
// the same audio-track sample-table consistency this security fix
// added everywhere else in that file.

// TestValidate_MP4ExtraOrUnvalidatedTracks covers the gap a
// differential security review found: FFmpeg's own mov demuxer opens
// and decodes every track whose codec fourcc it recognizes, whatever
// this package's own handler-type judgment made of it, and still
// parses a track that follows a malformed one rather than stopping
// there. A playable file must now declare exactly the tracks this
// package has validated and nothing else.
func TestValidate_MP4ExtraOrUnvalidatedTracks(t *testing.T) {
	t.Parallel()

	videoTrak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})
	audioTrak := mp4Trak(trakParams{handler: "soun", codec: "mp4a", timescale: 1000, durationUnit: 5000})
	hintTrak := mp4Trak(trakParams{handler: "hint", codec: "xxxx", timescale: 1000, durationUnit: 5000})
	malformedTrak := mp4Box("trak", []byte("not a mdia box"))

	moovWith := func(traks ...[]byte) []byte {
		mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
		body := append([]byte{}, mvhd...)
		for _, trak := range traks {
			body = append(body, trak...)
		}
		out := mp4Box("ftyp", []byte("isom0000isomiso2mp41"))
		return append(out, mp4Box("moov", body)...)
	}

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"video and one audio track, the baseline this still allows": {
			moovWith(videoTrak, audioTrak),
			true,
		},
		"video track with an extra hint track": {
			moovWith(videoTrak, hintTrak),
			false,
		},
		"video track with two audio tracks": {
			moovWith(videoTrak, audioTrak, mp4Trak(trakParams{handler: "soun", codec: "mp4a", timescale: 1000, durationUnit: 5000})),
			false,
		},
		"video track followed by a malformed trak": {
			moovWith(videoTrak, malformedTrak),
			false,
		},
		"video track followed by a trak header declaring a size past its container": {
			// The exact gap the review found in this package's own
			// children(): a box header that cannot even be read must
			// refuse the whole file, never silently fall back to just
			// the tracks already found before it - FFmpeg's own mov
			// demuxer shrinks an oversized box to fit rather than stop,
			// and still parses whatever follows it.
			func() []byte {
				oversizedTrakHeader := make([]byte, 8)
				binary.BigEndian.PutUint32(oversizedTrakHeader[0:4], 999_999)
				copy(oversizedTrakHeader[4:8], "trak")
				return moovWith(videoTrak, oversizedTrakHeader)
			}(),
			false,
		},
		"fragmented MP4: a moof box alongside a valid moov": {
			append(moovWith(videoTrak), mp4Box("moof", []byte("fragment"))...),
			false,
		},
		"fragmented MP4: an mfra box alongside a valid moov": {
			append(moovWith(videoTrak), mp4Box("mfra", []byte("fragment index"))...),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "tracks.mp4", tc.data)
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

// mp4ManyVideoTracks builds an moov box with n independent video
// traks, each otherwise identical to mp4Fixture's own, so a count
// above maxVideoTracks can be exercised directly.
func mp4ManyVideoTracks(n int) []byte {
	moovBody := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	for range n {
		moovBody = append(moovBody, mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 640, height: 480, timescale: 1000, durationUnit: 5000})...)
	}
	return mp4Box("moov", moovBody)
}
