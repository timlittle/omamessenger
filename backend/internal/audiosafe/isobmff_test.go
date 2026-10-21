package audiosafe_test

// isobmff_test.go checks Validate against MP4/M4A fixtures (see
// helpers_test.go's mp4AudioFixture) and fuzzes the same box walker
// against arbitrary bytes, so a crafted or merely truncated file can
// never make it panic, hang or report a verdict this package did not
// actually check for.

import (
	"encoding/binary"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MP4Audio covers a well-formed M4A file and every way
// this package's own limits reject one: an unsupported audio codec,
// a video track present at all, a second audio track, and a zero
// duration.
func TestValidate_MP4Audio(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"well-formed aac":       {mp4AudioFixture(mp4AudioParams{timescale: 1000, durationUnit: 5000}), true},
		"unsupported codec":     {mp4AudioFixture(mp4AudioParams{audioCodec: "xxxx", timescale: 1000, durationUnit: 5000}), false},
		"carries a video track": {mp4AudioFixture(mp4AudioParams{videoCodec: "avc1", timescale: 1000, durationUnit: 5000}), false},
		"a second audio track":  {mp4AudioFixture(mp4AudioParams{secondAudio: true, timescale: 1000, durationUnit: 5000}), false},
		"zero duration":         {mp4AudioFixture(mp4AudioParams{timescale: 1000, durationUnit: 0}), false},
		"no moov box at all":    {[]byte("not an mp4 file"), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "fixture.m4a", tc.data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestValidate_MP4AudioTooLong covers a duration over maxDuration.
func TestValidate_MP4AudioTooLong(t *testing.T) {
	t.Parallel()

	data := mp4AudioFixture(mp4AudioParams{timescale: 1, durationUnit: 60 * 31})
	path := writeFixture(t, "toolong.m4a", data)

	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a 31-minute file, want false")
	}
}

// TestValidate_MP4BoxHeaderVariants covers boxHeader's own less
// common encodings and failure paths, each reached through the
// public Validate function: a box using the 64-bit extended size, a
// box extending to the end of its container, a box too short to even
// hold a header, a box whose declared size cannot fit in its
// container, an mdhd using version 1 (64-bit timestamps), a
// truncated hdlr box, and a truncated stsd box.
func TestValidate_MP4BoxHeaderVariants(t *testing.T) {
	t.Parallel()

	mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
	trak := mp4Trak("soun", "mp4a", 1000, 5000)
	moovBody := append(append([]byte{}, mvhd...), trak...)

	extendedSizeMoov := make([]byte, 16+len(moovBody))
	binary.BigEndian.PutUint32(extendedSizeMoov[0:4], 1)
	copy(extendedSizeMoov[4:8], "moov")
	binary.BigEndian.PutUint64(extendedSizeMoov[8:16], uint64(16+len(moovBody)))
	copy(extendedSizeMoov[16:], moovBody)

	zeroSizeMoov := make([]byte, 8+len(moovBody))
	copy(zeroSizeMoov[4:8], "moov")
	copy(zeroSizeMoov[8:], moovBody)

	mdhdV1 := mp4Box("mdhd", mp4MediaHeaderV1(1000, 5000))
	hdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
	stsd := mp4Box("stsd", mp4SampleDescription("mp4a"))
	stbl := mp4Box("stbl", stsd)
	minf := mp4Box("minf", stbl)
	mdiaV1 := mp4Box("mdia", append(append(append([]byte{}, mdhdV1...), hdlr...), minf...))
	trakV1 := mp4Box("trak", mdiaV1)
	moovV1 := append(append([]byte{}, mvhd...), trakV1...)

	oversizeBox := make([]byte, 8)
	binary.BigEndian.PutUint32(oversizeBox[0:4], 1000)
	copy(oversizeBox[4:8], "moov")

	cases := map[string][]byte{
		"extended 64-bit size":       append(mp4Box("ftyp", []byte("isom")), extendedSizeMoov...),
		"size zero to container end": append(mp4Box("ftyp", []byte("isom")), zeroSizeMoov...),
		"box too short for header":   {0, 0, 0, 1},
		"box size exceeds container": oversizeBox,
		"mdhd version 1":             append(mp4Box("ftyp", []byte("isom")), mp4Box("moov", moovV1)...),
		"truncated hdlr": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov",
			append(append([]byte{}, mvhd...), mp4Box("trak", mp4Box("mdia",
				append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", []byte{0})...)))...))...),
		"truncated stsd": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov",
			append(append([]byte{}, mvhd...), mp4Box("trak", mp4Box("mdia",
				append(append(mp4Box("mdhd", mp4MediaHeader(1000, 5000)), mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))...),
					mp4Box("minf", mp4Box("stbl", mp4Box("stsd", []byte{1, 2})))...)))...))...),
		"trak with no mdia at all": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov",
			append(append([]byte{}, mvhd...), mp4Box("trak", []byte{})...))...),
		"no mvhd at all, trak duration carries it": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov", trak)...),
		"mdhd too short to read": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov",
			append(append([]byte{}, mvhd...), mp4Box("trak", mp4Box("mdia", mp4Box("mdhd", []byte{0, 0})))...))...),
		"hint track alongside a valid audio track": append(mp4Box("ftyp", []byte("isom")), mp4Box("moov",
			append(append(append([]byte{}, mvhd...), trak...), mp4Trak("hint", "", 1000, 5000)...))...),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "variant.m4a", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			wantTrue := map[string]bool{
				"mdhd version 1":                           true,
				"extended 64-bit size":                     true,
				"size zero to container end":               true,
				"no mvhd at all, trak duration carries it": true,
				"hint track alongside a valid audio track": true,
			}
			if wantTrue[name] {
				if !got {
					t.Errorf("Validate() = false for %q, want true", name)
				}
				return
			}
			if got {
				t.Errorf("Validate() = true for %q, want false", name)
			}
		})
	}
}

// FuzzValidateMP4Audio seeds the corpus with the crafted cases a
// header parser must survive without panicking, hanging or reading
// past its own bounds. go test's mutation engine takes it from there.
func FuzzValidateMP4Audio(f *testing.F) {
	f.Add(mp4AudioFixture(mp4AudioParams{timescale: 1000, durationUnit: 5000}))
	f.Add(mp4AudioFixture(mp4AudioParams{videoCodec: "avc1", timescale: 1000, durationUnit: 5000}))
	f.Add(mp4AudioFixture(mp4AudioParams{audioCodec: "xxxx", timescale: 1000, durationUnit: 5000}))
	f.Add(mp4AudioFixture(mp4AudioParams{timescale: 1000, durationUnit: 5000})[:40])
	f.Add([]byte("not an audio file at all"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.m4a", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
