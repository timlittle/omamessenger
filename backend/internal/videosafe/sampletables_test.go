package videosafe_test

// sampletables_test.go covers trackTablesAndEditsOK's own cross-checks
// (isobmff.go, sampletables.go): a security review found this
// package read an audio track's own sample count but never checked
// it against anything else at all, and that none of stts, ctts,
// stss, sgpd, sbgp or a trak's own edit list were read for either
// kind of track - exactly the tables FFmpeg's own mov demuxer sizes
// an allocation from. Every case here uses an audio track, so the
// mechanism under test is isolated from avc1's own in-band NAL scan,
// which never runs for one; TestValidate_MP4VideoTrackSampleTables
// below confirms the same checks also apply to a video track's own
// tables.

import (
	"encoding/binary"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// baselineAudioTables is one sample, in one chunk, with every other
// table omitted: the minimal, self-consistent stsz/stco/stsc triangle
// every case below starts from and then varies.
func baselineAudioTables() sampleTableParams {
	return sampleTableParams{
		stsz: stszBody(100),
		stco: stcoBody(0),
		stsc: stscBody([2]uint32{1, 1}),
	}
}

// TestValidate_MP4AudioSampleCountExceedsCap reproduces the exact
// shape a security review's own exploit scenario builds: an audio
// track (a voice note, in the review's own scenario) declaring a
// sample count far above maxSampleCount. Before this fix, nothing
// ever checked an audio track's own sample count against any limit
// at all.
func TestValidate_MP4AudioSampleCountExceedsCap(t *testing.T) {
	t.Parallel()

	const absurdCount = 85_000_000
	p := sampleTableParams{
		stsz: fixedStszBody(1, absurdCount), // sample_size=1, sample_count=85,000,000: a few bytes, no real per-sample table
	}
	data := mp4AudioTrackWithTables(p)

	path := writeFixture(t, "audiosamplecount.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an audio track declaring 85,000,000 samples, want false")
	}
}

// TestValidate_MP4AudioSTTSMismatch reproduces the review's own
// exploit mechanism directly: a small, capped stsz sample count (so
// the track passes maxSampleCount on its own) alongside a single stts
// entry whose own sample_count field claims a far larger total -
// exactly the field FFmpeg's own mov_build_index allocates
// sample_count*24 bytes from, independently of stsz's own count.
func TestValidate_MP4AudioSTTSMismatch(t *testing.T) {
	t.Parallel()

	p := baselineAudioTables()
	p.stts = sttsBody([2]uint32{85_000_000, 1024}) // one entry, 85M samples

	data := mp4AudioTrackWithTables(p)
	path := writeFixture(t, "sttsmismatch.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an stts total disagreeing with stsz's own sample count, want false")
	}
}

// TestValidate_MP4AudioSampleTableConsistency covers every other
// cross-check trackTablesAndEditsOK applies to an audio track's own
// stts, ctts, stss, sbgp and sgpd tables, each varied one at a time
// against the same one-sample baseline.
func TestValidate_MP4AudioSampleTableConsistency(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate func(p *sampleTableParams)
		want   bool
	}{
		"no extra tables at all": {
			func(p *sampleTableParams) {},
			true,
		},
		"stts summing to the declared sample count": {
			func(p *sampleTableParams) { p.stts = sttsBody([2]uint32{1, 1024}) },
			true,
		},
		"stts summing to more samples than declared": {
			func(p *sampleTableParams) { p.stts = sttsBody([2]uint32{2, 1024}) },
			false,
		},
		"ctts summing to the declared sample count": {
			func(p *sampleTableParams) { p.ctts = cttsBody([2]uint32{1, 0}) },
			true,
		},
		"ctts summing to more samples than declared": {
			func(p *sampleTableParams) { p.ctts = cttsBody([2]uint32{1, 0}, [2]uint32{1, 0}) },
			false,
		},
		"stss naming the one sample that exists": {
			func(p *sampleTableParams) { p.stss = stssBody(1) },
			true,
		},
		"stss naming a sample number past the declared count": {
			func(p *sampleTableParams) { p.stss = stssBody(2) },
			false,
		},
		"stss with more entries than samples": {
			func(p *sampleTableParams) { p.stss = stssBody(1, 2) },
			false,
		},
		"sbgp summing to the declared sample count": {
			func(p *sampleTableParams) { p.sbgp = sbgpBody([2]uint32{1, 1}) },
			true,
		},
		"sbgp summing to more samples than declared": {
			func(p *sampleTableParams) { p.sbgp = sbgpBody([2]uint32{2, 1}) },
			false,
		},
		"sgpd with one ordinary roll-type description": {
			func(p *sampleTableParams) { p.sgpd = sgpdBody(1) },
			true,
		},
		"sgpd declaring more descriptions than the box actually holds": {
			func(p *sampleTableParams) {
				body := sgpdBody(1)
				binary.BigEndian.PutUint32(body[12:16], 2) // entry_count lied about; only one description's own bytes are present
				p.sgpd = body
			},
			false,
		},
		"sbgp version 1": {
			func(p *sampleTableParams) { p.sbgp = sbgpBodyV1(1, 1) },
			true,
		},
		"sgpd with default_length 0, one variable-length description": {
			func(p *sampleTableParams) {
				entries := []byte{0, 0, 0, 1, 0xAB} // 4-byte length (1), then 1 byte of payload
				p.sgpd = sgpdBodyVariableLength(entries, 1)
			},
			true,
		},
		"sgpd with default_length 0, a description length past the box": {
			func(p *sampleTableParams) {
				entries := []byte{0, 0, 0, 5, 0xAB} // declares length 5, only 1 byte present
				p.sgpd = sgpdBodyVariableLength(entries, 1)
			},
			false,
		},
		"stts box too short to even hold entry_count": {
			func(p *sampleTableParams) { p.stts = []byte{0, 0, 0, 0} },
			false,
		},
		"stss box too short to even hold entry_count": {
			func(p *sampleTableParams) { p.stss = []byte{0, 0, 0, 0} },
			false,
		},
		"sbgp box too short to even hold its own grouping_type": {
			func(p *sampleTableParams) { p.sbgp = []byte{0, 0, 0, 0} },
			false,
		},
		"sbgp declares one entry but the box holds none": {
			func(p *sampleTableParams) { p.sbgp = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1} }, // version 0, entry_count 1, no row
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
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t, for %s", got, tc.want, name)
			}
		})
	}
}

// TestValidate_MP4VideoTrackSampleTables confirms
// trackTablesAndEditsOK's own cross-checks apply to a video track's
// own tables too, not only an audio track's: before this fix, only
// the video track's own sample count was ever bounded at all, and
// none of these other tables were checked for either kind.
func TestValidate_MP4VideoTrackSampleTables(t *testing.T) {
	t.Parallel()

	sample := avcSample(buildAVCSliceNAL(5))
	p := avcCustomLayoutParams{
		width: 1280, height: 720,
		stsz: stszBody(uint32(len(sample))),
		stco: stcoBody(0),
		stsc: stscBody([2]uint32{1, 1}),
		stts: sttsBody([2]uint32{85_000_000, 1024}), // mismatched total, same exploit shape
		mdat: sample,
	}
	data := mp4AVCVideoCustomLayout(p)
	patchStcoOffset(data, int64(len(data)-len(p.mdat)))

	path := writeFixture(t, "videostts.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a video track whose stts total disagrees with stsz's own sample count, want false")
	}
}

// TestValidate_MP4AudioStz2Refused covers stbl's less common stz2
// box on an audio track declaring a non-zero sample count: whose own
// variable-width per-sample table this package does not support, the
// same reason a video track with real samples already refuses it
// (see TestValidate_MP4InBandSampleLayoutVariants's "stz2 instead of
// stsz" case). This test used to assert the opposite, back when
// nothing checked an audio track's own sample tables at all; it now
// proves the two kinds of track are refused the same way.
func TestValidate_MP4AudioStz2Refused(t *testing.T) {
	t.Parallel()

	withStz2 := func() []byte {
		mvhd := mp4Box("mvhd", mp4MediaHeader(1000, 5000))
		videoTrak := mp4Trak(trakParams{handler: "vide", codec: "avc1", width: 1280, height: 720, timescale: 1000, durationUnit: 5000})

		audioMdhd := mp4Box("mdhd", mp4MediaHeader(1000, 5000))
		audioHdlr := mp4Box("hdlr", append(make([]byte, 8), []byte("soun")...))
		audioStsd := mp4Box("stsd", mp4SampleDescription("mp4a", 0, 0))
		stz2 := mp4Box("stz2", mp4SampleSize(42))
		audioStbl := mp4Box("stbl", append(append([]byte{}, audioStsd...), stz2...))
		audioMinf := mp4Box("minf", audioStbl)
		audioMdia := mp4Box("mdia", append(append(append([]byte{}, audioMdhd...), audioHdlr...), audioMinf...))
		audioTrak := mp4Box("trak", audioMdia)

		moovBody := append(append(append([]byte{}, mvhd...), videoTrak...), audioTrak...)
		return mp4Box("moov", moovBody)
	}()

	path := writeFixture(t, "stz2.mp4", withStz2)
	got, err := videosafe.Validate(path)
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
		"one entry far beyond maxDuration":      {edtsWith(elstBody(1, 1000*3600)), false}, // 3600s at a 1000Hz timescale
		"version 1, one in-range entry":         {edtsWith(elstBodyV1(5000)), true},
		"version 1, far beyond maxDuration":     {edtsWith(elstBodyV1(1000 * 3600)), false},
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
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Validate() = %t, want %t, for %s", got, tc.want, name)
			}
		})
	}
}
