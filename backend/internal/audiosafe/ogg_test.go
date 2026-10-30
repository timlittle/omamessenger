package audiosafe_test

// ogg_test.go checks Validate against Ogg/Opus fixtures and fuzzes
// the same page walker against arbitrary bytes, so a crafted or
// merely truncated file can never make it panic, hang or report a
// verdict this package did not actually check for.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_Ogg covers a well-formed single-page Opus stream and
// every way this package's own limits reject one: a second logical
// bitstream serial, a first page that is not really Opus, a granule
// position implying a duration over the cap, and no granule position
// (meaning duration zero) at all.
func TestValidate_Ogg(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		data []byte
		want bool
	}{
		"well-formed opus":           {oggOpusFixture(oggOpusParams{granule: 48_000 * 60}), true},
		"multiplexed second serial":  {oggOpusFixture(oggOpusParams{pageCount: 2, secondSerial: true, granule: 48_000 * 60}), false},
		"not actually opus":          {oggOpusFixture(oggOpusParams{skipOpusHead: true, granule: 48_000 * 60}), false},
		"duration over the cap":      {oggOpusFixture(oggOpusParams{granule: 48_000 * 60 * 31}), false},
		"no granule position at all": {oggOpusFixture(oggOpusParams{granule: 0}), false},
		"chained stream after the first one's own EOS": {
			// The exact gap a differential security review found: the
			// old code broke out of its own page loop the moment it saw
			// the first stream's EOS page, so a second, distinct
			// bitstream appended right after it - which could carry
			// video - was never even read, let alone judged. FFmpeg's
			// own Ogg demuxer still opens a chained stream that starts
			// there, so this package must keep reading to the end of
			// the file.
			append(
				oggPage(0x02|0x04, 48_000*60, 1, 0, []byte("OpusHead further opus header bytes here")),
				oggPage(0x02|0x04, 48_000*60, 2, 0, []byte("OpusHead further opus header bytes here"))...,
			),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "fixture.ogg", tc.data)
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

// TestValidate_OggStructuralEdgeCases covers validateOgg's own
// remaining rejection paths: a first page not marked BOS at all, a
// page's own declared segment lengths summing past the file's real
// end (so the walk never lands exactly on it, closing the same gap
// the chained-stream fix above does for a plain truncation rather
// than a second stream), and a later page whose own capture pattern
// is not "OggS" at all.
func TestValidate_OggStructuralEdgeCases(t *testing.T) {
	t.Parallel()

	notBOS := oggPage(0x04, 48_000*60, 1, 0, []byte("OpusHead further opus header bytes here"))

	overrunPage := oggPage(0x02, 48_000*60, 1, 0, []byte("OpusHead"))
	overrunPage[27] = 100 // claims a 100-byte segment; only 8 bytes of payload actually follow

	corruptedSecondPage := append(
		oggPage(0x02, 0, 1, 0, []byte("OpusHead further opus header bytes here")),
		[]byte("not an Ogg page at all")...,
	)

	cases := map[string][]byte{
		"first page not marked BOS":                      notBOS,
		"segment lengths sum past the file's own end":    overrunPage,
		"a later page's capture pattern is not \"OggS\"": corruptedSecondPage,
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "edge.ogg", data)
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

// TestValidate_OggTruncated covers a page header, and a segment
// table, each cut off partway through: neither must ever read past
// the file's own end or report a false positive.
func TestValidate_OggTruncated(t *testing.T) {
	t.Parallel()

	full := oggOpusFixture(oggOpusParams{granule: 48_000 * 60})

	cases := map[string][]byte{
		"truncated page header":   full[:10],
		"truncated segment table": full[:27],
		"truncated payload":       full[:30],
		"empty file":              {},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "truncated.ogg", data)
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

// FuzzValidateOgg seeds the corpus with the crafted cases a page
// walker must survive without panicking, hanging or reading past its
// own bounds. go test's mutation engine takes it from there.
func FuzzValidateOgg(f *testing.F) {
	f.Add(oggOpusFixture(oggOpusParams{granule: 48_000 * 60}))
	f.Add(oggOpusFixture(oggOpusParams{pageCount: 2, secondSerial: true, granule: 48_000 * 60}))
	f.Add(oggOpusFixture(oggOpusParams{skipOpusHead: true, granule: 48_000 * 60}))
	f.Add(oggOpusFixture(oggOpusParams{granule: 48_000 * 60})[:20])
	f.Add(append(
		oggPage(0x02|0x04, 48_000*60, 1, 0, []byte("OpusHead further opus header bytes here")),
		oggPage(0x02|0x04, 48_000*60, 2, 0, []byte("OpusHead further opus header bytes here"))...,
	))
	f.Add([]byte("OggS"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		path := writeFixture(t, "fuzz.ogg", data)
		if _, err := audiosafe.Validate(path); err != nil {
			t.Fatalf("Validate() error = %v, want nil for a file that opened fine", err)
		}
	})
}
