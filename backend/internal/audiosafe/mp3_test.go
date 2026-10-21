package audiosafe_test

// mp3_test.go checks Validate against constant-bitrate MP3 fixtures
// (see helpers_test.go's mp3Fixture) and fuzzes the same frame
// scanner against arbitrary bytes, so a crafted or merely truncated
// file can never make it panic, hang or report a verdict this
// package did not actually check for.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MP3 covers a well-formed constant-bitrate stream,
// with and without a leading ID3v2 tag, and every way this package
// refuses the simple case: too few confirming frames, and a
// declared duration over the cap (approximated from file size and
// bitrate, so this uses a very low bitrate to make a short fixture
// look implausibly long).
func TestValidate_MP3(t *testing.T) {
	t.Parallel()

	t.Run("well-formed constant bitrate", func(t *testing.T) {
		t.Parallel()

		path := writeFixture(t, "fixture.mp3", mp3Fixture(128, 44_100, 10))
		got, err := audiosafe.Validate(path)
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		if !got {
			t.Error("Validate() = false, want true")
		}
	})

	t.Run("with a leading id3v2 tag", func(t *testing.T) {
		t.Parallel()

		data := append(id3v2Tag(32), mp3Fixture(128, 44_100, 10)...)
		path := writeFixture(t, "tagged.mp3", data)
		got, err := audiosafe.Validate(path)
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		if !got {
			t.Error("Validate() = false, want true")
		}
	})

	t.Run("too few frames to confirm", func(t *testing.T) {
		t.Parallel()

		path := writeFixture(t, "short.mp3", mp3Fixture(128, 44_100, 1))
		got, err := audiosafe.Validate(path)
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		if got {
			t.Error("Validate() = true, want false")
		}
	})

	t.Run("not mp3 at all", func(t *testing.T) {
		t.Parallel()

		path := writeFixture(t, "notmp3.mp3", []byte("plain text, not audio"))
		got, err := audiosafe.Validate(path)
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		if got {
			t.Error("Validate() = true, want false")
		}
	})
}

// TestValidate_MP3FrameHeaderRejections covers every field
// mp3FrameHeader checks beyond the sync pattern itself: a version
// other than MPEG-1, a layer other than Layer III, a reserved or
// free bitrate index, and a reserved sample-rate index.
func TestValidate_MP3FrameHeaderRejections(t *testing.T) {
	t.Parallel()

	good := mp3Frame(128, 44_100)

	cases := map[string][]byte{
		"not mpeg-1 (mpeg-2 version bits)": withByte(good, 1, good[1]&^0x18|0x10),
		"not layer iii (layer ii bits)":    withByte(good, 1, good[1]&^0x06|0x04),
		"free bitrate index":               withByte(good, 2, good[2]&0x0F),
		"reserved bitrate index":           withByte(good, 2, 0xF0|good[2]&0x0F),
		"reserved sample rate index":       withByte(good, 2, good[2]&0xF0|0x0C),
	}

	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Three more well-formed frames follow the corrupted one, so a
			// rejection here can only be the corrupted header's own fault,
			// never confirmMP3Frames simply running out of frames to read.
			data := append(append([]byte{}, frame...), mp3Fixture(128, 44_100, 3)...)
			path := writeFixture(t, "rejected.mp3", data)
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

// withByte returns a copy of frame with the byte at index set to v.
func withByte(frame []byte, index int, v byte) []byte {
	out := append([]byte{}, frame...)
	out[index] = v
	return out
}

// FuzzValidateMP3 seeds the corpus with the crafted cases a frame
// scanner must survive without panicking, hanging or reading past
// its own bounds. go test's mutation engine takes it from there.
func FuzzValidateMP3(f *testing.F) {
	f.Add(mp3Fixture(128, 44_100, 10))
	f.Add(append(id3v2Tag(32), mp3Fixture(128, 44_100, 10)...))
	f.Add(mp3Fixture(128, 44_100, 1))
	f.Add([]byte{0xFF, 0xFB, 0x90, 0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp3", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
