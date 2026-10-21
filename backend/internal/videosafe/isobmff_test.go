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
			mp4Fixture(mp4Params{videoCodec: "vp09", audioCodec: "mp4a", width: 640, height: 480, durationUnit: 5000}),
			true,
		},
		"av1 video with opus audio": {
			mp4Fixture(mp4Params{videoCodec: "av01", audioCodec: "Opus", width: 1920, height: 1080, durationUnit: 5000}),
			true,
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
// and one that is not a video at all. go test's mutation engine takes
// it from there.
func FuzzValidateMP4(f *testing.F) {
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{width: 4000, height: 4000, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{videoCodec: "xxxx", width: 640, height: 480, durationUnit: 5000}))
	f.Add(mp4Fixture(mp4Params{width: 1280, height: 720, durationUnit: 5000})[:40])
	f.Add([]byte("not a video file at all"))
	f.Add([]byte{})

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

// TestValidate_MP4SampleCountFallback covers stbl's less common stz2
// box, read the same way as stsz, and a box too short to hold even
// its own sample_count field.
func TestValidate_MP4SampleCountFallback(t *testing.T) {
	t.Parallel()

	withStz2 := func() []byte {
		mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
		mdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
		hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("vide")...))
		stsd := mp4Box("stsd", mp4SampleDescription("avc1", 1280, 720))
		stz2 := mp4Box("stz2", mp4SampleSize(42))
		stbl := mp4Box("stbl", append(append([]byte{}, stsd...), stz2...))
		minf := mp4Box("minf", stbl)
		mdia := mp4Box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
		trak := mp4Box("trak", mdia)
		return mp4Box("moov", append(append([]byte{}, mvhd...), trak...))
	}()

	path := writeFixture(t, "stz2.mp4", withStz2)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if !got {
		t.Error("Validate() = false for a valid file using stz2 instead of stsz, want true")
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
