package videosafe_test

// sampleentry_test.go covers sampleentry.go: a security review found
// avcConfigDetails located avcC by type alone (firstChild), never
// checking what other boxes the same avc1 entry might carry, and that
// neither this package's mp4a nor Opus entry was checked past its own
// codec fourcc at all. These tests reproduce both gaps directly:
// a replacement decoder configuration (esds, hvcC or a second avcC)
// sitting alongside avc1's own avcC, and an mp4a esds describing
// something other than AAC.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4AVC1SampleEntryChildren covers avcSampleEntryChildrenOK:
// avcC exactly once, the small set of boxes real encoders also write
// alongside it, and every shape a replacement configuration could
// take - an esds, an hvcC (whether after or, the exploit's own
// ordering, before avcC), and a second avcC.
func TestValidate_MP4AVC1SampleEntryChildren(t *testing.T) {
	t.Parallel()

	const w, h = 1280, 720
	avcc := mp4Box("avcC", buildAVCDecoderConfig(w, h))

	cases := map[string]struct {
		extra []byte
		want  bool
	}{
		"avcC alone": {nil, true},
		"avcC with pasp, colr, btrt and fiel": {
			concatBytes(mp4Box("pasp", []byte{0, 0, 0, 1, 0, 0, 0, 1}), mp4Box("colr", []byte("nclx")), mp4Box("btrt", make([]byte, 12)), mp4Box("fiel", []byte{1, 0})),
			true,
		},
		"an esds alongside avcC": {mp4Box("esds", []byte("not a real esds")), false},
		"an hvcC alongside avcC": {mp4Box("hvcC", []byte("not a real hvcC")), false},
		"a second avcC":          {avcc, false},
		"an unrecognized box":    {mp4Box("zzzz", []byte("unknown")), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			entryBody := make([]byte, 78)
			binaryPutDims(entryBody, w, h)
			entryBody = append(entryBody, avcc...)
			if name == "an hvcC alongside avcC" {
				// The exploit's own ordering: a replacement
				// configuration placed *before* the trusted avcC,
				// which FFmpeg's own mov demuxer reads first.
				entryBody = append(append(make([]byte, 78), tc.extra...), avcc...)
				binaryPutDims(entryBody, w, h)
			} else {
				entryBody = append(entryBody, tc.extra...)
			}

			data := buildCustomAVCFixtureWithEntryBody(uint16(w), uint16(h), entryBody)
			path := writeFixture(t, "avc1children.mp4", data)
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

// TestValidate_MP4a_EsdsAAC covers mp4aSampleEntryOK and
// esdsDescribesAAC: exactly one esds, objectTypeIndication 0x40, and
// an AudioSpecificConfig whose audioObjectType, sample rate and
// channel configuration this package allows.
func TestValidate_MP4a_EsdsAAC(t *testing.T) {
	t.Parallel()

	aacLC44100Stereo := mp4Box("esds", buildAACESDS(2, 4, 2))

	cases := map[string]struct {
		children []byte
		want     bool
	}{
		"ordinary AAC LC, 44.1kHz, stereo": {aacLC44100Stereo, true},
		"AAC LC, 8kHz, mono":               {mp4Box("esds", buildAACESDS(2, 11, 1)), true},
		"HE-AAC (SBR)":                     {mp4Box("esds", buildAACESDS(5, 4, 2)), true},
		"no esds at all":                   {nil, false},
		"two esds boxes":                   {concatBytes(aacLC44100Stereo, aacLC44100Stereo), false},
		"an unrecognized extra box":        {concatBytes(aacLC44100Stereo, mp4Box("chan", []byte("x"))), false},
		"esds with a btrt alongside it, as real ffmpeg AAC encodes carry": {
			concatBytes(aacLC44100Stereo, mp4Box("btrt", make([]byte, 12))),
			true,
		},
		"esds with a non-AAC object type":                 {mp4Box("esds", buildESDSWithObjectTypeIndication(0x6B)), false}, // 0x6B: MPEG-1 audio
		"esds with a disallowed AAC object":               {mp4Box("esds", buildAACESDS(1, 4, 2)), false},                   // 1: AAC Main, never written by a real encoder this package has seen
		"esds with a reserved sample rate index":          {mp4Box("esds", buildAACESDS(2, 13, 2)), false},
		"esds with channel config 0":                      {mp4Box("esds", buildAACESDS(2, 4, 0)), false},
		"esds too short to hold an ES_Descriptor":         {mp4Box("esds", []byte{0, 0, 0, 0}), false},
		"esds with an explicit, in-range sample rate":     {mp4Box("esds", buildAACESDSExplicitRate(2, 48_000, 2)), true},
		"esds with an explicit sample rate below 8kHz":    {mp4Box("esds", buildAACESDSExplicitRate(2, 4_000, 2)), false},
		"esds with an explicit sample rate above 192kHz":  {mp4Box("esds", buildAACESDSExplicitRate(2, 200_000, 2)), false},
		"esds with an explicit rate but channel config 0": {mp4Box("esds", buildAACESDSExplicitRate(2, 48_000, 0)), false},
		"esds with a DecoderConfigDescriptor carrying no DecoderSpecificInfo at all": {
			mp4Box("esds", func() []byte {
				dcd := []byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // exactly 13 bytes, no DSI
				dcdDesc := mp4Descriptor(0x04, dcd)
				es := append([]byte{0, 0, 0}, dcdDesc...)
				esDesc := mp4Descriptor(0x03, es)
				return append([]byte{0, 0, 0, 0}, esDesc...)
			}()),
			false,
		},
		"esds padded past the single-byte descriptor length encoding": {
			mp4Box("esds", buildAACESDSPadded(2, 4, 2, 150)),
			true,
		},
		"esds whose ES_Descriptor length never terminates": {
			mp4Box("esds", append([]byte{0, 0, 0, 0}, 0x03, 0x80, 0x80, 0x80, 0x80)),
			false,
		},
		"esds too short to even hold version/flags": {
			mp4Box("esds", []byte{0, 0}),
			false,
		},
		"esds longer than maxESDSLength": {
			mp4Box("esds", buildAACESDSPadded(2, 4, 2, 2000)),
			false,
		},
		"esds whose DecoderConfigDescriptor tag is wrong": {
			mp4Box("esds", func() []byte {
				dcd := append([]byte{0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, mp4Descriptor(0x05, []byte{0x12, 0x10})...)
				dcdDesc := mp4Descriptor(0x06, dcd) // wrong tag: not 0x04
				es := append([]byte{0, 0, 0}, dcdDesc...)
				esDesc := mp4Descriptor(0x03, es)
				return append([]byte{0, 0, 0, 0}, esDesc...)
			}()),
			false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := mp4FixtureWithAudioEntry("mp4a", tc.children)
			path := writeFixture(t, "mp4a.mp4", data)
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

// TestValidate_MP4AudioEntryTooShort covers audioSampleEntry's own
// length guard: an entry shorter than audioSampleEntryFixedFields
// (28 bytes) is refused before any child box is even looked for.
func TestValidate_MP4AudioEntryTooShort(t *testing.T) {
	t.Parallel()

	data := mp4FixtureWithTinyAudioEntry("mp4a", 10)
	path := writeFixture(t, "tinyaudioentry.mp4", data)
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for an audio sample entry shorter than its own fixed fields, want false")
	}
}

// TestValidate_MP4OpusSampleEntry covers opusSampleEntryOK: exactly
// one dOps box, version 0, and nothing else.
func TestValidate_MP4OpusSampleEntry(t *testing.T) {
	t.Parallel()

	dops := mp4Box("dOps", buildOpusSpecificBox())

	cases := map[string]struct {
		children []byte
		want     bool
	}{
		"ordinary dOps":                         {dops, true},
		"no dOps at all":                        {nil, false},
		"two dOps boxes":                        {concatBytes(dops, dops), false},
		"an unrecognized extra":                 {concatBytes(dops, mp4Box("esds", []byte("x"))), false},
		"dOps with a btrt alongside it":         {concatBytes(dops, mp4Box("btrt", make([]byte, 12))), true},
		"dOps with version 1":                   {mp4Box("dOps", append([]byte{1}, make([]byte, 10)...)), false},
		"dOps too short to hold a version byte": {mp4Box("dOps", []byte{}), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := mp4FixtureWithAudioEntry("Opus", tc.children)
			path := writeFixture(t, "opus.mp4", data)
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
