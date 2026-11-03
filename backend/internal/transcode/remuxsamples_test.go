package transcode

// remuxsamples_test.go tests remuxsamples.go directly, as an
// internal test: these cases are all about per-sample validation
// and the scratch-file plumbing around it, reached the same way as
// remux_test.go's own cases (see its header comment for why this is
// an internal test rather than going through the public API).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

func TestRemuxMP4_SampleRefusals(t *testing.T) {
	t.Parallel()

	runRemuxRefusals(t, map[string]remuxRefusal{
		"nonzero ctts": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			setCtts(t, video, 3, 1)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"zero duration sample": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			video.Mdia.Minf.Stbl.Stts.SampleTimeDelta[0] = 0
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"first sample is not an IDR": {build: func(t *testing.T, dir string) string {
			samples := [][]byte{nalSample(1), nalSample(1), nalSample(1)}
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, samples)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{samples})
		}},
		"sample has an in-band SPS NAL": {build: func(t *testing.T, dir string) string {
			samples := [][]byte{nalSample(5, 7), nalSample(1), nalSample(1)}
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, samples)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{samples})
		}},
		"sample has an unrecognised NAL type": {build: func(t *testing.T, dir string) string {
			samples := [][]byte{nalSample(5), nalSample(20), nalSample(1)}
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, samples)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{samples})
		}},
		"sample is empty after filtering": {build: func(t *testing.T, dir string) string {
			samples := [][]byte{nalSample(5), nalSample(6, 9), nalSample(1)}
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, samples)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{samples})
		}},
		"too many video samples": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.maxVideoSamples = 2; return &c }(),
		},
		"no video samples at all": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				video.Mdia.Minf.Stbl.Stsz.SampleNumber = 0
				video.Mdia.Minf.Stbl.Stsz.SampleSize = nil
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
			},
		},
		"video sample over its size cap": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.maxVideoSampleLen = 2; return &c }(),
		},
		"zero duration audio sample": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
			audio.Mdia.Minf.Stbl.Stts.SampleTimeDelta[0] = 0
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"nonzero composition time offset on an audio sample": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
			setCtts(t, audio, 3, 2)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"too many audio samples": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.maxAudioSamples = 2; return &c }(),
		},
		"audio sample over its size cap": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.maxAudioSampleLen = 1; return &c }(),
		},
		"declared sample range reaches past the file": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			video.Mdia.Minf.Stbl.Stsz.SampleSize[0] = 1 << 20
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"rebuilt output would exceed its total size cap": {
			// Each of the three default samples is 8 bytes once
			// rebuilt (a 4-byte length prefix plus its own 4-byte
			// NAL unit); a cap of 10 lets the first sample through
			// but must refuse once the second one would push the
			// running total to 16 - proving the cap sums video's
			// own samples as they are copied, not just checking a
			// single one of them against it, which nothing else
			// here already does.
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.maxOutputBytes = 10; return &c }(),
		},
		"scratch file already exists": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			in := writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
			if err := os.WriteFile(filepath.Join(dir, "out.mp4.scratch"), []byte("already here"), 0o600); err != nil {
				t.Fatalf("pre-creating scratch file: %v", err)
			}
			return in
		}},
	})
}

func TestFilterVideoSample(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sample  []byte
		want    []byte
		wantIDR bool
		wantErr bool
	}{
		"IDR alone":                 {sample: nalSample(5), want: nalSample(5), wantIDR: true},
		"non-IDR alone":             {sample: nalSample(1), want: nalSample(1)},
		"IDR with SEI and AUD":      {sample: nalSample(9, 5, 6), want: nalSample(5), wantIDR: true},
		"non-IDR with AUD":          {sample: nalSample(9, 1), want: nalSample(1)},
		"empty after filtering":     {sample: nalSample(6, 9), wantErr: true},
		"in-band SPS refused":       {sample: nalSample(5, 7), wantErr: true},
		"in-band PPS refused":       {sample: nalSample(5, 8), wantErr: true},
		"unrecognised type refused": {sample: nalSample(20), wantErr: true},
		"malformed length refused":  {sample: []byte{0, 0, 0, 99, 5}, wantErr: true},
		"empty sample refused":      {sample: nil, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, isIDR, err := filterVideoSample(tt.sample)
			if tt.wantErr {
				if !errors.Is(err, ErrFailed) {
					t.Fatalf("filterVideoSample() error = %v, want %v", err, ErrFailed)
				}
				return
			}
			if err != nil {
				t.Fatalf("filterVideoSample() error = %v, want nil", err)
			}
			if string(got) != string(tt.want) {
				t.Errorf("filterVideoSample() = %x, want %x", got, tt.want)
			}
			if isIDR != tt.wantIDR {
				t.Errorf("isIDR = %v, want %v", isIDR, tt.wantIDR)
			}
		})
	}
}

func TestScratchWriter_WriteError(t *testing.T) {
	t.Parallel()

	// A closed file's own Write always fails: the simplest way to
	// reach write's own error branch without needing a real
	// disk-full or permission failure.
	f, err := os.CreateTemp(t.TempDir(), "scratch-*")
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing fixture: %v", err)
	}

	sw := &scratchWriter{f: f, max: 1 << 20}
	if err := sw.write([]byte("x")); !errors.Is(err, ErrFailed) {
		t.Fatalf("write() error = %v, want %v", err, ErrFailed)
	}
}

// TestScratchWriter_RefusesOnceTheRunningTotalWouldExceedMax covers
// the cap itself, directly: two writes that are each individually
// under max but whose combined total is not.
func TestScratchWriter_RefusesOnceTheRunningTotalWouldExceedMax(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "scratch-*")
	if err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
	defer f.Close()

	sw := &scratchWriter{f: f, max: 10}
	if err := sw.write([]byte("12345")); err != nil {
		t.Fatalf("first write() error = %v, want nil", err)
	}
	if err := sw.write([]byte("678901")); !errors.Is(err, ErrFailed) {
		t.Fatalf("second write() error = %v, want %v once the running total would exceed max", err, ErrFailed)
	}

	got, readErr := os.ReadFile(f.Name())
	if readErr != nil {
		t.Fatalf("reading scratch file: %v", readErr)
	}
	if string(got) != "12345" {
		t.Errorf("scratch file = %q, want only the first write's own bytes: the refused second write must never reach disk at all", got)
	}
}
