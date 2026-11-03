package transcode

// internal_helpers_test.go holds the test fixtures remux_test.go,
// remuxsamples_test.go and remuxbuild_test.go all share: building an
// adversarial MP4 with mp4ff by hand is common to most of what they
// test, and this package's own test support for the public API
// already claims the name helpers_test.go (package transcode_test,
// a different package in the same directory), so this file - for
// this internal test package's own shared support - needs a
// different one.

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"
)

// mainSPSHex/mainPPSHex are one real AVC Main profile, level 3.1,
// 320x240 SPS/PPS pair - exactly the shape remuxMP4 accepts -
// generated once with the host's own ffmpeg (never through this
// package) and hardcoded here so these tests need no ffmpeg at all.
// The others each break exactly one field checkSPS looks at.
const (
	mainSPSHex        = "674d401feca0a0fd8088000003000800000300a078c18cb0"
	mainPPSHex        = "68ebe3cb20"
	largeSPSHex       = "674d4028eca03c0113f2e022000003000200000300281e30632c" // Main, level 4.0, 1920x1080: over remuxMaxDimension
	highProfileSPSHex = "6764001facd94141fb011000000300100000030140f1831960"   // High profile, not Main
	highProfilePPSHex = "68ebe3cb22c0"
	level51SPSHex     = "674d4033eca0a0fd8088000003000800000300a078c18cb0" // level 5.1, over spsMaxLevel
)

// fataler is the small subset of *testing.T's API the fixture
// builders below actually call: both *testing.T and *testing.F
// satisfy it, so FuzzRemux can build its own seed corpus with the
// same builders an ordinary test uses.
type fataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

// mustHex decodes s, failing t if it is not valid hex.
func mustHex(t fataler, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decoding fixture hex %q: %v", s, err)
	}
	return b
}

// nalSample builds one length-prefixed sample from the given NAL
// unit types, each with a few arbitrary payload bytes: remuxMP4
// never decodes slice data, only reads a NAL's own type byte, so
// the payload's actual content never matters to it.
func nalSample(types ...int) []byte {
	var out []byte
	for _, nt := range types {
		nalu := []byte{byte(nt), 0xaa, 0xbb, 0xcc}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(nalu)))
		out = append(out, length[:]...)
		out = append(out, nalu...)
	}
	return out
}

// buildVideoTrakForTest builds a one-entry avc1 video trak for
// testing: avcC from spsHex/ppsHex, with samples (each already NAL-
// length-prefixed, see nalSample) at 1000 ticks each over a 10000
// timescale.
func buildVideoTrakForTest(t fataler, spsHex, ppsHex string, samples [][]byte) *mp4.TrakBox {
	t.Helper()
	trak := mp4.CreateEmptyTrak(1, 10000, "video", "und")
	sps, pps := mustHex(t, spsHex), mustHex(t, ppsHex)
	if err := trak.SetAVCDescriptor("avc1", [][]byte{sps}, [][]byte{pps}, true); err != nil {
		t.Fatalf("SetAVCDescriptor: %v", err)
	}
	durs := make([]uint32, len(samples))
	for i := range durs {
		durs[i] = 1000
	}
	fillTestSampleTables(t, trak, samples, durs)
	return trak
}

// buildAudioTrakForTest builds a one-entry mp4a audio trak for
// testing, with samples at 1024 ticks each (one AAC-LC frame) over a
// timescale equal to sampleRate.
func buildAudioTrakForTest(t fataler, objectType byte, sampleRate int, channelCfg byte, samples [][]byte) *mp4.TrakBox {
	t.Helper()
	trak := mp4.CreateEmptyTrak(2, uint32(sampleRate), "audio", "und")
	if err := trak.SetAACDescriptorWithChannelConfig(objectType, channelCfg, sampleRate); err != nil {
		t.Fatalf("SetAACDescriptorWithChannelConfig: %v", err)
	}
	durs := make([]uint32, len(samples))
	for i := range durs {
		durs[i] = 1024
	}
	fillTestSampleTables(t, trak, samples, durs)
	return trak
}

// fillTestSampleTables builds trak's own stts/stsc/stsz/stco
// directly from samples and durs, with a placeholder chunk offset:
// writeTestMP4File fills in the real one once it knows the file's
// own layout.
func fillTestSampleTables(t fataler, trak *mp4.TrakBox, samples [][]byte, durs []uint32) {
	t.Helper()
	stbl := trak.Mdia.Minf.Stbl
	stts := &mp4.SttsBox{}
	sizes := make([]uint32, len(samples))
	for i, s := range samples {
		stts.SampleCount = append(stts.SampleCount, 1)
		stts.SampleTimeDelta = append(stts.SampleTimeDelta, durs[i])
		sizes[i] = uint32(len(s))
	}
	stbl.Stts = stts
	stbl.Stsc = &mp4.StscBox{}
	if err := stbl.Stsc.AddEntry(1, uint32(len(samples)), 1); err != nil {
		t.Fatalf("stsc.AddEntry: %v", err)
	}
	stbl.Stsz = &mp4.StszBox{SampleNumber: uint32(len(samples)), SampleSize: sizes}
	stbl.Stco = &mp4.StcoBox{ChunkOffset: []uint32{0}}
	stbl.Children = []mp4.Box{stbl.Stsd, stbl.Stts, stbl.Stsc, stbl.Stsz, stbl.Stco}
}

// setCtts gives every sample of trak the same composition time
// offset, for a test that needs a nonzero or all-zero ctts.
func setCtts(t fataler, trak *mp4.TrakBox, nrSamples int, offset int32) {
	t.Helper()
	ctts := &mp4.CttsBox{}
	if err := ctts.AddSampleCountsAndOffset([]uint32{uint32(nrSamples)}, []int32{offset}); err != nil {
		t.Fatalf("AddSampleCountsAndOffset: %v", err)
	}
	stbl := trak.Mdia.Minf.Stbl
	stbl.Ctts = ctts
	stbl.Children = []mp4.Box{stbl.Stsd, stbl.Stts, ctts, stbl.Stsc, stbl.Stsz, stbl.Stco}
}

// writeTestMP4File assembles ftyp+moov+mdat from traks and their own
// samples, one chunk per track in the same order as traks, and
// writes it to dir/name.
func writeTestMP4File(t fataler, dir, name string, traks []*mp4.TrakBox, allSamples [][][]byte) string {
	t.Helper()
	moov := mp4.NewMoovBox()
	moov.AddChild(mp4.CreateMvhd())
	for _, trak := range traks {
		moov.AddChild(trak)
	}
	moov.Mvhd.Timescale = 10000
	moov.Mvhd.NextTrackID = uint32(len(moov.Traks) + 1)

	ftyp := mp4.NewFtyp(mp4.BrandIsom, 0, []string{mp4.BrandIso2, mp4.BrandAvc1, mp4.BrandMp41})
	offset := int64(ftyp.Size()) + int64(moov.Size()) + 8
	var payload int64
	for i, trak := range traks {
		trak.Mdia.Minf.Stbl.Stco.ChunkOffset[0] = uint32(offset)
		n := totalLen(allSamples[i])
		offset += n
		payload += n
	}

	path := filepath.Join(dir, name)
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	defer func() { _ = out.Close() }()

	if err := ftyp.Encode(out); err != nil {
		t.Fatalf("encoding ftyp: %v", err)
	}
	if err := moov.Encode(out); err != nil {
		t.Fatalf("encoding moov: %v", err)
	}
	if err := mp4.EncodeHeaderWithSize("mdat", uint64(payload)+8, false, out); err != nil {
		t.Fatalf("encoding mdat header: %v", err)
	}
	for _, samples := range allSamples {
		for _, s := range samples {
			if _, err := out.Write(s); err != nil {
				t.Fatalf("writing sample: %v", err)
			}
		}
	}
	return path
}

// totalLen adds up the length of every sample in samples.
func totalLen(samples [][]byte) int64 {
	var n int64
	for _, s := range samples {
		n += int64(len(s))
	}
	return n
}

// defaultVideoSamples returns three valid samples: an IDR followed
// by two non-IDR slices, the shape every happy-path test builds on.
func defaultVideoSamples() [][]byte {
	return [][]byte{nalSample(5), nalSample(1), nalSample(1)}
}

// defaultAudioSamples returns three arbitrary AAC-shaped samples:
// remuxMP4 never decodes audio content, only validates its esds and
// per-sample size and duration.
func defaultAudioSamples() [][]byte {
	return [][]byte{{0xaa, 0xbb}, {0xaa, 0xbb}, {0xaa, 0xbb}}
}

// defaultCaps is a remuxCaps generous enough for every fixture these
// tests build.
func defaultCaps() remuxCaps {
	return remuxCaps{
		allowAudio:        true,
		maxVideoSamples:   100,
		maxAudioSamples:   100,
		maxVideoSampleLen: 1 << 20,
		maxAudioSampleLen: 1 << 20,
		maxOutputBytes:    10 << 20,
	}
}

// mustOpen opens path, failing t on error; the file is left open,
// which is fine for a short-lived test process.
func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	return f
}

// remuxRefusal is one named case for runRemuxRefusals: build returns
// a fixture path, and caps, when non-nil, overrides defaultCaps()
// for this one case.
type remuxRefusal struct {
	build func(t *testing.T, dir string) string
	caps  *remuxCaps
}

// runRemuxRefusals runs every case in tests and confirms remuxMP4
// reports ErrFailed for each one.
func runRemuxRefusals(t *testing.T, tests map[string]remuxRefusal) {
	t.Helper()
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			in := tt.build(t, dir)
			caps := defaultCaps()
			if tt.caps != nil {
				caps = *tt.caps
			}

			err := remuxMP4(in, filepath.Join(dir, "out.mp4"), caps)
			if !errors.Is(err, ErrFailed) {
				t.Fatalf("remuxMP4() error = %v, want %v", err, ErrFailed)
			}
		})
	}
}
