package transcode

// remuxbuild_test.go tests remuxbuild.go directly, as an internal
// test: confirming the exact bytes and structure a rebuild produces
// needs mp4ff's own decoder on the result, which is easiest to drive
// against the hand-built fixtures remux_test.go's own cases use (see
// its header comment for why this is an internal test rather than
// going through the public API).

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

func TestRemuxMP4_ValidRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	videoSamples, audioSamples := defaultVideoSamples(), defaultAudioSamples()
	video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, videoSamples)
	setCtts(t, video, len(videoSamples), 0) // an all-zero ctts must be accepted, not refused
	audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, audioSamples)
	in := writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{videoSamples, audioSamples})

	out := filepath.Join(dir, "out.mp4")
	if err := remuxMP4(in, out, defaultCaps()); err != nil {
		t.Fatalf("remuxMP4() error = %v", err)
	}

	parsed, err := mp4.DecodeFile(mustOpen(t, out))
	if err != nil {
		t.Fatalf("re-parsing rebuilt file: %v", err)
	}
	checkRebuiltStructure(t, parsed, videoSamples, audioSamples)
}

// mustOpenNotExist confirms path does not exist, failing t
// otherwise: used to confirm a refused rebuild leaves no output
// file behind at all.
func mustOpenNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want a not-exist error", path, err)
	}
}

// checkRebuiltStructure confirms parsed has exactly the two tracks
// this package always writes, no edit list on either, and that the
// video track's own samples are wantVideo's own NAL content minus
// nothing (every sample here is already slice-only), with the audio
// track's own samples exactly wantAudio's bytes.
func checkRebuiltStructure(t *testing.T, parsed *mp4.File, wantVideo, wantAudio [][]byte) {
	t.Helper()
	if len(parsed.Moov.Traks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(parsed.Moov.Traks))
	}
	video, audio := parsed.Moov.Traks[0], parsed.Moov.Traks[1]
	if video.Edts != nil || audio.Edts != nil {
		t.Error("rebuilt file carries an edit list; want none")
	}

	gotVideo, err := parsed.ReadSampleData(video, 1, video.GetNrSamples(), nil, nil)
	if err != nil {
		t.Fatalf("reading rebuilt video samples: %v", err)
	}
	if !bytes.Equal(gotVideo, bytes.Join(wantVideo, nil)) {
		t.Errorf("rebuilt video sample bytes = %x, want %x", gotVideo, bytes.Join(wantVideo, nil))
	}

	gotAudio, err := parsed.ReadSampleData(audio, 1, audio.GetNrSamples(), nil, nil)
	if err != nil {
		t.Fatalf("reading rebuilt audio samples: %v", err)
	}
	if !bytes.Equal(gotAudio, bytes.Join(wantAudio, nil)) {
		t.Errorf("rebuilt audio sample bytes = %x, want %x", gotAudio, bytes.Join(wantAudio, nil))
	}
}

func TestRemuxMP4_DropsSEIAndAUDButKeepsDurations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	videoSamples := [][]byte{nalSample(5, 6, 9), nalSample(1, 6), nalSample(9, 1)}
	video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, videoSamples)
	in := writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{videoSamples})

	out := filepath.Join(dir, "out.mp4")
	caps := defaultCaps()
	caps.allowAudio = false
	if err := remuxMP4(in, out, caps); err != nil {
		t.Fatalf("remuxMP4() error = %v", err)
	}

	parsed, err := mp4.DecodeFile(mustOpen(t, out))
	if err != nil {
		t.Fatalf("re-parsing rebuilt file: %v", err)
	}
	video2 := parsed.Moov.Traks[0]
	want := bytes.Join([][]byte{nalSample(5), nalSample(1), nalSample(1)}, nil)
	got, err := parsed.ReadSampleData(video2, 1, video2.GetNrSamples(), nil, nil)
	if err != nil {
		t.Fatalf("reading rebuilt samples: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("rebuilt samples = %x, want %x (SEI and AUD dropped)", got, want)
	}

	durs, err := video2.Mdia.Minf.Stbl.Stts.SampleDurations(video2.GetNrSamples())
	if err != nil {
		t.Fatalf("reading rebuilt durations: %v", err)
	}
	for i, d := range durs {
		if d != 1000 {
			t.Errorf("sample %d duration = %d, want 1000", i+1, d)
		}
	}
}

func TestRemuxMP4_RefusalLeavesNoOutputFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	samples := [][]byte{nalSample(1), nalSample(1), nalSample(1)} // no IDR: refused
	video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, samples)
	in := writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{samples})
	out := filepath.Join(dir, "out.mp4")

	if err := remuxMP4(in, out, defaultCaps()); !errors.Is(err, ErrFailed) {
		t.Fatalf("remuxMP4() error = %v, want %v", err, ErrFailed)
	}
	mustOpenNotExist(t, out)
	mustOpenNotExist(t, out+".scratch")
}

func TestSumDurations(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		durs []uint32
		want int64
	}{
		"empty":   {durs: nil, want: 0},
		"one":     {durs: []uint32{1000}, want: 1000},
		"several": {durs: []uint32{1000, 2000, 3000}, want: 6000},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sumDurations(tt.durs); got != tt.want {
				t.Errorf("sumDurations(%v) = %d, want %d", tt.durs, got, tt.want)
			}
		})
	}
}

func TestRescaleTicks(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dur      int64
		from, to uint32
		want     int64
	}{
		"same timescale":          {dur: 1000, from: 48000, to: 48000, want: 1000},
		"zero source is a no-op":  {dur: 1000, from: 0, to: 48000, want: 1000},
		"upscale":                 {dur: 1024, from: 48000, to: 96000, want: 2048},
		"downscale with rounding": {dur: 1000, from: 48000, to: 44100, want: 919}, // 1000*44100/48000 = 918.75, rounds to 919
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := rescaleTicks(tt.dur, tt.from, tt.to); got != tt.want {
				t.Errorf("rescaleTicks(%d, %d, %d) = %d, want %d", tt.dur, tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// boundedFailWriter fails the write that would push the total bytes
// written past limit, succeeding (and counting) every byte before
// that: it lets a test choose exactly which of encodeOutput's own
// sequential writes (ftyp, then moov, then the mdat header, then the
// copied scratch bytes) is the one that fails.
type boundedFailWriter struct {
	limit   int
	written int
}

func (w *boundedFailWriter) Write(p []byte) (int, error) {
	if w.written+len(p) > w.limit {
		return 0, errors.New("boundedFailWriter: limit reached")
	}
	w.written += len(p)
	return len(p), nil
}

func TestEncodeOutput_WriteFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scratchPath := filepath.Join(dir, "scratch")
	if err := os.WriteFile(scratchPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("writing scratch fixture: %v", err)
	}

	ftyp := mp4.NewFtyp(mp4.BrandIsom, 0, []string{mp4.BrandIso2, mp4.BrandAvc1, mp4.BrandMp41})
	moov := mp4.NewMoovBox()
	moov.AddChild(mp4.CreateMvhd())

	ftypSize := int(ftyp.Size())
	moovSize := int(moov.Size())

	tests := map[string]int{
		"ftyp write fails":        0,
		"moov write fails":        ftypSize,
		"mdat header write fails": ftypSize + moovSize,
		"scratch copy fails":      ftypSize + moovSize + mdatHeaderBytes,
	}
	for name, limit := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := &boundedFailWriter{limit: limit}
			err := encodeOutput(w, ftyp, moov, scratchPath, 7)
			if !errors.Is(err, ErrFailed) {
				t.Fatalf("encodeOutput() error = %v, want %v", err, ErrFailed)
			}
		})
	}
}

func TestCopyScratch_OpenError(t *testing.T) {
	t.Parallel()

	err := copyScratch(io.Discard, filepath.Join(t.TempDir(), "missing"), 10)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("copyScratch() error = %v, want %v", err, ErrFailed)
	}
}

func TestCopyScratch_ShorterThanWanted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "scratch")
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := copyScratch(io.Discard, path, 1000)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("copyScratch() error = %v, want %v", err, ErrFailed)
	}
}
