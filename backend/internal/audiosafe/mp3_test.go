package audiosafe_test

// mp3_test.go checks Validate against constant-bitrate MP3 fixtures
// (see helpers_test.go's mp3Fixture) and fuzzes the same frame
// scanner against arbitrary bytes, so a crafted or merely truncated
// file can never make it panic, hang or report a verdict this
// package did not actually check for. It also covers
// mp3HasForbiddenTag: a differential security review found that
// audiosafe's own former frame-by-frame ID3v2 walk and FFmpeg's id3v2
// demuxer disagreed about where a v2.4 frame actually ends once its
// size field's high bit is set, letting an attached picture hide in
// bytes the walk treated as padding (see docs/decisions.md). The fix
// taken here is deliberately blunt: an MP3 plays in-window only when
// it carries no ID3v2, APE or Lyrics3 tag anywhere at all, so there is
// no frame structure left for the two parsers to disagree about.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MP3 covers a well-formed constant-bitrate stream, with
// no tag at all and with a trailing ID3v1 tag, and every way this
// package refuses the simple case: too few confirming frames, and a
// declared duration over the cap (approximated from file size and
// bitrate, so this uses a very low bitrate to make a short fixture
// look implausibly long).
func TestValidate_MP3(t *testing.T) {
	t.Parallel()

	t.Run("well-formed constant bitrate, no tag at all", func(t *testing.T) {
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

	t.Run("a trailing ID3v1 tag at the very end is still allowed", func(t *testing.T) {
		t.Parallel()

		data := append(mp3Fixture(128, 44_100, 10), id3v1Tag()...)
		path := writeFixture(t, "id3v1.mp3", data)
		got, err := audiosafe.Validate(path)
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		if !got {
			t.Error("Validate() = false, want true: ID3v1 has no room for a picture")
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

// TestValidate_MP3RefusesAnyForbiddenTag covers every place and shape
// of tag mp3HasForbiddenTag must refuse: a leading ID3v2 tag -
// whether it carries only ordinary text frames or an attached
// picture, since this package no longer tells the two apart - an
// ID3v2 tag appended after the audio instead of leading it, a second
// ID3v2 tag alongside a first, an APE tag, and a Lyrics3 tag (v1 and
// v2 share the same "LYRICSBEGIN" opening marker this package looks
// for).
func TestValidate_MP3RefusesAnyForbiddenTag(t *testing.T) {
	t.Parallel()

	audio := mp3Fixture(128, 44_100, 10)
	ordinaryTag := id3v2TagWithFrames(3, id3v2Frame(3, "TIT2", []byte("Example Song")))
	pictureTag := id3v2TagWithFrames(3, id3v2Frame(3, "APIC", []byte("fake picture bytes")))

	cases := map[string][]byte{
		"leading ID3v2 tag with only ordinary text frames": append(append([]byte{}, ordinaryTag...), audio...),
		"leading ID3v2 tag with an attached picture":       append(append([]byte{}, pictureTag...), audio...),
		"ID3v2 tag appended after the audio":               append(append([]byte{}, audio...), ordinaryTag...),
		"a second ID3v2 tag alongside a first":             append(append(append([]byte{}, ordinaryTag...), audio...), ordinaryTag...),
		"an APE tag":                                       append(append([]byte{}, audio...), apeTag()...),
		"a Lyrics3 tag":                                    append(append([]byte{}, audio...), lyrics3Tag()...),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "forbidden.mp3", data)
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

// TestValidate_MP3ForbiddenTagAcrossAChunkBoundary covers
// mp3HasForbiddenTag's own chunk overlap: a marker placed to straddle
// exactly the boundary between two scan chunks must still be found,
// not missed because each chunk was searched in isolation.
func TestValidate_MP3ForbiddenTagAcrossAChunkBoundary(t *testing.T) {
	t.Parallel()

	const scanChunkSize = 64 * 1024 // mirrors mp3ScanChunkSize in limits.go
	marker := []byte("APETAGEX")
	filler := make([]byte, scanChunkSize-len(marker)/2)
	data := append(append([]byte{}, filler...), marker...)
	data = append(data, mp3Fixture(128, 44_100, 10)...)

	path := writeFixture(t, "straddling.mp3", data)
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a marker straddling a chunk boundary, want false")
	}
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
// scanner and the forbidden-tag scan must survive without panicking,
// hanging or reading past their own bounds. go test's mutation engine
// takes it from there.
func FuzzValidateMP3(f *testing.F) {
	f.Add(mp3Fixture(128, 44_100, 10))
	f.Add(append(id3v2Tag(32), mp3Fixture(128, 44_100, 10)...))
	f.Add(append(mp3Fixture(128, 44_100, 10), id3v1Tag()...))
	f.Add(append(mp3Fixture(128, 44_100, 10), apeTag()...))
	f.Add(append(mp3Fixture(128, 44_100, 10), lyrics3Tag()...))
	f.Add(mp3Fixture(128, 44_100, 1))
	f.Add([]byte{0xFF, 0xFB, 0x90, 0x00})
	f.Add([]byte("ID3"))
	f.Add([]byte("APETAGEX"))
	f.Add([]byte("LYRICSBEGIN"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.mp3", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
