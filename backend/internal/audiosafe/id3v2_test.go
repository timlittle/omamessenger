package audiosafe_test

// id3v2_test.go checks Validate against ID3v2 tags carrying a
// forbidden frame (the exact bypass a differential security review
// found: a leading ID3v2 tag was skipped by its own declared size,
// never read, so an attached picture or encapsulated object it
// carried reached QtMultimedia's own decoder unexamined) and fuzzes
// the same frame walk against arbitrary bytes, so a crafted or merely
// truncated tag can never make it panic, hang or report a verdict
// this package did not actually check for.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MP3RefusesID3v2AttachedPicture covers every ID3v2
// frame id and tag version this package treats as carrying an
// embedded picture or object: APIC and GEOB in ID3v2.3 and ID3v2.4,
// and their three-letter ID3v2.2 equivalents PIC and GEO.
func TestValidate_MP3RefusesID3v2AttachedPicture(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		version byte
		frameID string
	}{
		"ID3v2.2 PIC":  {2, "PIC"},
		"ID3v2.2 GEO":  {2, "GEO"},
		"ID3v2.3 APIC": {3, "APIC"},
		"ID3v2.3 GEOB": {3, "GEOB"},
		"ID3v2.4 APIC": {4, "APIC"},
		"ID3v2.4 GEOB": {4, "GEOB"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tag := id3v2TagWithFrames(tc.version, id3v2Frame(tc.version, tc.frameID, []byte("fake picture or object bytes")))
			data := append(tag, mp3Fixture(128, 44_100, 10)...)
			path := writeFixture(t, "picture.mp3", data)

			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got {
				t.Errorf("Validate() = true for a tag carrying %q, want false", name)
			}
		})
	}
}

// TestValidate_MP3AllowsOrdinaryID3v2Frames covers the counterpart to
// TestValidate_MP3RefusesID3v2AttachedPicture: a tag carrying only
// ordinary text frames (title, artist), in every version this package
// walks, must still validate as playable.
func TestValidate_MP3AllowsOrdinaryID3v2Frames(t *testing.T) {
	t.Parallel()

	for _, version := range []byte{2, 3, 4} {
		t.Run(map[byte]string{2: "ID3v2.2", 3: "ID3v2.3", 4: "ID3v2.4"}[version], func(t *testing.T) {
			t.Parallel()

			titleID, artistID := "TIT2", "TPE1"
			if version == 2 {
				titleID, artistID = "TT2", "TP1"
			}

			tag := id3v2TagWithFrames(version,
				id3v2Frame(version, titleID, []byte("Example Song")),
				id3v2Frame(version, artistID, []byte("Example Artist")),
			)
			data := append(tag, mp3Fixture(128, 44_100, 10)...)
			path := writeFixture(t, "ordinary.mp3", data)

			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if !got {
				t.Error("Validate() = false, want true for an ordinary ID3v2 tag")
			}
		})
	}
}

// TestValidate_MP3RefusesID3v2EdgeCases covers every other way
// id3v2Scan refuses a tag rather than risk mis-locating a frame it
// cannot confirm is not a forbidden one: an extended header flag this
// package does not parse, a frame id containing a character no real
// ID3v2 frame id ever does, and a frame whose own declared size runs
// past the tag's own end.
func TestValidate_MP3RefusesID3v2EdgeCases(t *testing.T) {
	t.Parallel()

	extendedHeader := make([]byte, 10)
	copy(extendedHeader[0:3], "ID3")
	extendedHeader[3], extendedHeader[4] = 4, 0 // version 2.4.0
	extendedHeader[5] = 0x40                    // extended header flag set
	copy(extendedHeader[6:10], synchsafeEncode(0))

	badFrameID := id3v2TagWithFrames(3, id3v2Frame(3, "A!IC", []byte("data")))

	// id3v2TagWithFrames(3, ...)'s 10-byte tag header is followed
	// immediately by this one frame's own 4-byte id, then its 4-byte
	// size field at absolute offset 14-17; setting its low byte well
	// past the body's real 5-byte length ("short") makes the frame
	// claim more bytes than the tag has left.
	overrunFrame := id3v2TagWithFrames(3, id3v2Frame(3, "APIC", []byte("short")))
	overrunFrame[17] = 0x7F

	cases := map[string][]byte{
		"extended header flag set":           append(extendedHeader, mp3Fixture(128, 44_100, 10)...),
		"frame id with an invalid character": append(badFrameID, mp3Fixture(128, 44_100, 10)...),
		"frame size runs past the tag's end": append(overrunFrame, mp3Fixture(128, 44_100, 10)...),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "edge.mp3", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got {
				t.Errorf("Validate() = true for %q, want false", name)
			}
		})
	}
}

// FuzzValidateMP3ID3v2 seeds the corpus with the crafted ID3v2 tags a
// frame walk must survive without panicking, hanging or reading past
// its own bounds. go test's mutation engine takes it from there.
func FuzzValidateMP3ID3v2(f *testing.F) {
	f.Add(append(id3v2TagWithFrames(3, id3v2Frame(3, "APIC", []byte("fake picture bytes"))), mp3Fixture(128, 44_100, 10)...))
	f.Add(append(id3v2TagWithFrames(2, id3v2Frame(2, "PIC", []byte("fake picture bytes"))), mp3Fixture(128, 44_100, 10)...))
	f.Add(append(id3v2TagWithFrames(4, id3v2Frame(4, "GEOB", []byte("fake object bytes"))), mp3Fixture(128, 44_100, 10)...))
	f.Add(append(id3v2TagWithFrames(3, id3v2Frame(3, "TIT2", []byte("Example Song"))), mp3Fixture(128, 44_100, 10)...))
	f.Add([]byte("ID3"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp3", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
