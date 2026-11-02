package audiosafe_test

// sampletables_test.go covers soundTablesAndEntryOK's own cross-checks
// (isobmff.go, sampletables.go, samplegrouptables.go): a security
// review's own exploit scenario was exactly this package's one audio
// track - a voice note - declaring a sample count far above any
// limit, with nothing checking it, its stts, ctts, stss, sgpd, sbgp
// or its trak's own edit list against anything at all (see
// docs/decisions.md). Mirrors videosafe's own sampletables_test.go.

import (
	"encoding/binary"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// baselineAudioTables is one sample, in one chunk, with every other
// table omitted: the minimal, self-consistent stsz/stco/stsc triangle
// every case below starts from and then varies.
func baselineAudioTables() audioTableParams {
	return audioTableParams{
		stsz: stszBody(100),
		stco: stcoBody(0),
		stsc: stscBody(1, 1),
	}
}

// TestValidate_MP4AudioSampleCountExceedsCap reproduces the exact
// shape a security review's own exploit scenario builds: a voice note
// declaring a sample count far above maxSampleCount. Before this fix,
// nothing ever checked this package's one audio track's own sample
// count against any limit at all.
func TestValidate_MP4AudioSampleCountExceedsCap(t *testing.T) {
	t.Parallel()

	const absurdCount = 85_000_000
	p := audioTableParams{
		stsz: fixedStszBody(1, absurdCount), // sample_size=1, sample_count=85,000,000: a few bytes, no real per-sample table
	}
	data := mp4AudioTrackWithTables(p)

	path := writeFixture(t, "audiosamplecount.mp4", data)
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an audio track declaring 85,000,000 samples, want false")
	}
}

// TestValidate_MP4AudioSTTSMismatch reproduces the review's own
// exploit mechanism directly: a small, capped stsz sample count
// alongside a single stts entry whose own sample_count field claims a
// far larger total - exactly the field FFmpeg's own mov_build_index
// allocates sample_count*24 bytes from, independently of stsz's own
// count.
func TestValidate_MP4AudioSTTSMismatch(t *testing.T) {
	t.Parallel()

	p := baselineAudioTables()
	p.stts = sttsBody(85_000_000, 1024) // one entry, 85M samples

	data := mp4AudioTrackWithTables(p)
	path := writeFixture(t, "sttsmismatch.mp4", data)
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an stts total disagreeing with stsz's own sample count, want false")
	}
}

// TestValidate_MP4AudioSampleTableConsistency covers every other
// cross-check soundTablesAndEntryOK applies to this package's one
// audio track's own stts, ctts, stss, sbgp and sgpd tables, each
// varied one at a time against the same one-sample baseline.
func TestValidate_MP4AudioSampleTableConsistency(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate func(p *audioTableParams)
		want   bool
	}{
		"no extra tables at all": {
			func(*audioTableParams) {},
			true,
		},
		"stts summing to the declared sample count": {
			func(p *audioTableParams) { p.stts = sttsBody(1, 1024) },
			true,
		},
		"stts summing to more samples than declared": {
			func(p *audioTableParams) { p.stts = sttsBody(2, 1024) },
			false,
		},
		"ctts summing to the declared sample count": {
			func(p *audioTableParams) { p.ctts = cttsBody(1, 0) },
			true,
		},
		"ctts summing to more samples than declared": {
			func(p *audioTableParams) { p.ctts = cttsBody(1, 0, 1, 0) },
			false,
		},
		"stss naming the one sample that exists": {
			func(p *audioTableParams) { p.stss = stssBody(1) },
			true,
		},
		"stss naming a sample number past the declared count": {
			func(p *audioTableParams) { p.stss = stssBody(2) },
			false,
		},
		"stss with more entries than samples": {
			func(p *audioTableParams) { p.stss = stssBody(1, 2) },
			false,
		},
		"sbgp summing to the declared sample count": {
			func(p *audioTableParams) { p.sbgp = sbgpBody(1, 1) },
			true,
		},
		"sbgp summing to more samples than declared": {
			func(p *audioTableParams) { p.sbgp = sbgpBody(2, 1) },
			false,
		},
		"sgpd with one ordinary roll-type description": {
			func(p *audioTableParams) { p.sgpd = sgpdBody(1) },
			true,
		},
		"sgpd declaring more descriptions than the box actually holds": {
			func(p *audioTableParams) {
				body := sgpdBody(1)
				binary.BigEndian.PutUint32(body[12:16], 2) // entry_count lied about; only one description's own bytes are present
				p.sgpd = body
			},
			false,
		},
		"sbgp version 1": {
			func(p *audioTableParams) { p.sbgp = sbgpBodyV1(1, 1) },
			true,
		},
		"sgpd with default_length 0, one variable-length description": {
			func(p *audioTableParams) {
				entries := []byte{0, 0, 0, 1, 0xAB} // 4-byte length (1), then 1 byte of payload
				p.sgpd = sgpdBodyVariableLength(entries, 1)
			},
			true,
		},
		"sgpd with default_length 0, a description length past the box": {
			func(p *audioTableParams) {
				entries := []byte{0, 0, 0, 5, 0xAB} // declares length 5, only 1 byte present
				p.sgpd = sgpdBodyVariableLength(entries, 1)
			},
			false,
		},
		"stts box too short to even hold entry_count": {
			func(p *audioTableParams) { p.stts = []byte{0, 0, 0, 0} },
			false,
		},
		"stss box too short to even hold entry_count": {
			func(p *audioTableParams) { p.stss = []byte{0, 0, 0, 0} },
			false,
		},
		"sbgp box too short to even hold its own grouping_type": {
			func(p *audioTableParams) { p.sbgp = []byte{0, 0, 0, 0} },
			false,
		},
		"sbgp declares one entry but the box holds none": {
			func(p *audioTableParams) { p.sbgp = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1} }, // version 0, entry_count 1, no row
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := baselineAudioTables()
			tc.mutate(&p)
			data := mp4AudioTrackWithTables(p)

			path := writeFixture(t, "tables.mp4", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t, for %s", got, tc.want, name)
			}
		})
	}
}

// TestValidate_MP4AudioSampleLayoutVariants covers sampleSpans' own
// stsz/stco-co64/stsc layout triangle, read fresh for this package's
// one audio track (mirroring videosafe's own avc1-track layout
// tests): a fixed sample size, 64-bit chunk offsets, several chunks
// across two stsc entries, and every malformed shape stscEntriesValid
// must refuse.
func TestValidate_MP4AudioSampleLayoutVariants(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		p    audioTableParams
		want bool
	}{
		"fixed sample size": {
			audioTableParams{stsz: fixedStszBody(10, 1), stco: stcoBody(0), stsc: stscBody(1, 1)},
			true,
		},
		"64-bit chunk offsets": {
			audioTableParams{stsz: stszBody(10), co64: co64Body(0), stsc: stscBody(1, 1)},
			true,
		},
		"two samples across two chunks": {
			audioTableParams{
				stsz: stszBody(10, 20),
				stco: stcoBody(0, 100),
				stsc: stscBodyTwoEntries(1, 1, 2, 1),
			},
			true,
		},
		"no stco or co64 at all": {
			audioTableParams{stsz: stszBody(10), stsc: stscBody(1, 1)},
			false,
		},
		"stco present, no stsc at all": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0)},
			false,
		},
		"stsz box too short to even hold its own header": {
			audioTableParams{stsz: []byte{0, 0, 0}},
			false,
		},
		"stsz declares two samples but stsc only ever places one": {
			audioTableParams{stsz: stszBody(10, 20), stco: stcoBody(0), stsc: stscBody(1, 1)},
			false,
		},
		"stsc not starting at chunk 1": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0), stsc: stscBody(2, 1)},
			false,
		},
		"stsc zero samples_per_chunk": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0), stsc: stscBody(1, 0)},
			false,
		},
		"stsc sample_description_index other than 1": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0), stsc: stscBodyWithIndex(1, 1, 2)},
			false,
		},
		"stsc entries out of order": {
			audioTableParams{
				stsz: stszBody(10, 20),
				stco: stcoBody(0, 100),
				stsc: stscBodyTwoEntries(2, 1, 1, 1), // chunk 2 declared before chunk 1
			},
			false,
		},
		"stco box too short to even hold entry_count": {
			audioTableParams{stsz: stszBody(10), stco: []byte{0, 0, 0}, stsc: stscBody(1, 1)},
			false,
		},
		"stco declares more offsets than the box actually holds": {
			audioTableParams{stsz: stszBody(10), stco: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0}, stsc: stscBody(1, 1)},
			false,
		},
		"stsc box too short to even hold entry_count": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0), stsc: []byte{0, 0, 0}},
			false,
		},
		"stsc declares more rows than the box actually holds": {
			audioTableParams{stsz: stszBody(10), stco: stcoBody(0), stsc: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1}},
			false,
		},
		"stsc duplicate first_chunk": {
			audioTableParams{
				stsz: stszBody(10, 20),
				stco: stcoBody(0, 100),
				stsc: stscBodyTwoEntries(1, 1, 1, 1),
			},
			false,
		},
		"stsz declares fewer per-sample entries than the box actually holds room for": {
			audioTableParams{stsz: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}, stco: stcoBody(0), stsc: stscBody(1, 1)}, // sample_count 2, no per-sample sizes at all
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := mp4AudioTrackWithTables(tc.p)

			path := writeFixture(t, "layout.mp4", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t, for %s", got, tc.want, name)
			}
		})
	}
}

// TestValidate_MP4AudioStz2Refused covers stbl's less common stz2
// box declaring a non-zero sample count: whose own variable-width
// per-sample table this package does not support. This test used to
// assert the opposite, back when nothing checked this package's one
// audio track's own sample tables at all.
func TestValidate_MP4AudioStz2Refused(t *testing.T) {
	t.Parallel()

	p := audioTableParams{stz2: mp4SampleSize(42)}
	data := mp4AudioTrackWithTables(p)

	path := writeFixture(t, "stz2.mp4", data)
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an audio track declaring samples through stz2, want false")
	}
}

// TestValidate_MP4EditList covers editListAllowed: at most one elst
// entry, and that one entry's own segment duration within maxDuration.
func TestValidate_MP4EditList(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		edts []byte
		want bool
	}{
		"no edts at all":                        {nil, true},
		"one in-range entry":                    {edtsWith(elstBody(1, 5000)), true},
		"two entries, not just one":             {edtsWith(elstBody(2, 5000)), false},
		"one entry far beyond maxDuration":      {edtsWith(elstBody(1, 1000*60*31)), false}, // 31 minutes at a 1000Hz timescale, over the 30-minute cap
		"version 1, one in-range entry":         {edtsWith(elstBodyV1(5000)), true},
		"version 1, far beyond maxDuration":     {edtsWith(elstBodyV1(1000 * 60 * 31)), false},
		"elst too short to hold its own header": {mp4Box("edts", mp4Box("elst", []byte{0, 0, 0})), false},
		"version 1, too short to hold its own 8-byte duration": {
			mp4Box("edts", mp4Box("elst", append([]byte{1, 0, 0, 0, 0, 0, 0, 1}, make([]byte, 4)...))),
			false,
		},
		"version 0, too short to hold its own 4-byte duration": {
			mp4Box("edts", mp4Box("elst", []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0})), // entry_count 1, 2 bytes of duration present
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := baselineAudioTables()
			p.edts = tc.edts
			data := mp4AudioTrackWithTables(p)

			path := writeFixture(t, "editlist.mp4", data)
			got, err := audiosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t, for %s", got, tc.want, name)
			}
		})
	}
}
