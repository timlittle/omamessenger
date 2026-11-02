package transcode_test

// helpers_test.go holds the test doubles every test file in this
// package needs: fakeRunner, a hand-written fake satisfying the
// small, unexported interface Transcoder.run actually calls, so a
// profile's own wiring - which output name, timeout and size cap it
// uses - and its own fixed ffmpeg argument list can both be checked
// through the public API alone, by inspecting what reached Run,
// rather than needing access to anything unexported; and
// blockingRunner, which holds Run open until a test lets it finish,
// for proving two jobs on the same Transcoder never run at once.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

// fakeRunner stands in for a *sandbox.Runner: Available and Run both
// return whatever a test configured, and Run also writes to the job's
// own pre-created spec.Output when writeOutput is set, simulating
// what a real ffmpeg run inside the sandbox would have left there -
// the real run already pre-creates that file itself (see
// joboutput.go) before any Runner.Run is ever called, exactly as the
// real sandbox.Runner would see it too.
type fakeRunner struct {
	availErr error

	result sandbox.Result
	runErr error

	writeOutput     bool
	writeOutputData []byte

	// writeOutputSymlinkTo, when set instead of writeOutputData, makes
	// Run replace the pre-created spec.Output with a symlink to this
	// target rather than an ordinary file, simulating a compromised or
	// misbehaving ffmpeg leaving a symlink where the sandbox told it to
	// write its real output. This no longer reflects something the
	// real sandbox can still produce, now that Run binds only
	// spec.Output's own single file rather than a writable directory
	// around it (see docs/decisions.md) - but checkOutput's own Lstat
	// defence stays in place regardless, so it is still worth proving
	// directly here.
	writeOutputSymlinkTo string

	// lastSpec is the Spec the most recent Run call received, for a
	// test to check a profile's own argv and paths against.
	lastSpec sandbox.Spec
}

// Available returns availErr, whatever a test set it to.
func (f *fakeRunner) Available(_ context.Context, _ string) error {
	return f.availErr
}

// Run records spec, optionally replaces the caller's own pre-created
// spec.Output with fake ffmpeg output, and returns result and runErr.
func (f *fakeRunner) Run(_ context.Context, spec sandbox.Spec) (sandbox.Result, error) {
	f.lastSpec = spec

	if f.runErr != nil {
		return sandbox.Result{}, f.runErr
	}

	if f.writeOutputSymlinkTo != "" {
		if err := os.Remove(spec.Output); err != nil {
			return sandbox.Result{}, err
		}
		if err := os.Symlink(f.writeOutputSymlinkTo, spec.Output); err != nil {
			return sandbox.Result{}, err
		}
	} else if f.writeOutput {
		data := f.writeOutputData
		if data == nil {
			data = []byte("fake ffmpeg output")
		}
		if err := os.WriteFile(spec.Output, data, 0o600); err != nil {
			return sandbox.Result{}, err
		}
	}

	return f.result, nil
}

// blockingRunner is a runner whose Run does not return until the test
// sends on unblock, for proving that Transcoder lets only one job run
// through it at a time. Available always succeeds immediately: only
// Run's own duration matters to that test. Safe to call Run on more
// than once (started only ever closes the first time), so the same
// instance can also be used to check that a slot it held is free
// again once unblock has already been closed.
type blockingRunner struct {
	startedOnce sync.Once
	// started is closed the first time Run is actually called, so a
	// test can wait for that rather than racing it with a sleep.
	started chan struct{}
	// unblock is read by Run; sending on it (or closing it) lets any
	// call currently blocked in Run return result.
	unblock chan struct{}
	result  sandbox.Result
}

// newBlockingRunner returns a blockingRunner with its own channels
// already made, ready to use.
func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}), unblock: make(chan struct{})}
}

// Available always succeeds.
func (b *blockingRunner) Available(context.Context, string) error { return nil }

// Run marks itself started, then blocks until unblock is sent to or
// closed, or ctx ends first.
func (b *blockingRunner) Run(ctx context.Context, _ sandbox.Spec) (sandbox.Result, error) {
	b.startedOnce.Do(func() { close(b.started) })

	select {
	case <-b.unblock:
		return b.result, nil
	case <-ctx.Done():
		return sandbox.Result{}, ctx.Err()
	}
}

// fakeInputFile creates a small file at dir/name with arbitrary
// content and returns its path: run's own job-input placement (see
// placeJobInput) links or copies a real file from disk, even for a
// test whose fakeRunner never looks at Spec.Input's own bytes.
func fakeInputFile(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake input"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// requireRealTranscoder returns a Transcoder backed by a real
// sandbox.Runner and the host's own ffmpeg, skipping the test unless
// the host can actually run a job end to end right now.
func requireRealTranscoder(t *testing.T) *transcode.Transcoder {
	t.Helper()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found on PATH")
	}

	r := sandbox.NewRunner(t.TempDir())
	if err := r.Available(t.Context(), ffmpegPath); err != nil {
		t.Skipf("sandbox unavailable on this host: %v", err)
	}

	return transcode.NewTranscoder(r, ffmpegPath)
}

// integrationDir returns a fresh directory under the repository's own
// build/ (never /tmp, see resources.md), three levels above this
// package, for an integration test's own fixtures and transcode
// output; it is removed once the test ends, pass or fail.
func integrationDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("../../../build", "transcode-it-*")
	if err != nil {
		t.Fatalf("creating build/ scratch directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // best-effort: a throwaway directory this test alone created

	return dir
}

// generateFixture runs the host's own ffmpeg - never this package's
// sandboxed one, which only ever runs through Transcoder - to build a
// small synthetic input file at filepath.Join(dir, name) from args,
// skipping the test if ffmpeg cannot produce it: an optional encoder,
// such as libx265, may not be built into the host's ffmpeg.
func generateFixture(t *testing.T, dir, name string, args ...string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	full := append(append([]string{"-y", "-v", "error"}, args...), path)

	cmd := exec.CommandContext(t.Context(), "ffmpeg", full...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not generate fixture %s: %v\n%s", name, err, out)
	}

	return path
}

// generateRotatedFixture builds a 1280x720 landscape clip, then
// remuxes it with a 90-degree display-matrix rotation injected
// through ffmpeg's own -display_rotation option. Confirmed that this
// option is read-side only - passing it directly to an encode's
// output is rejected - so the rotation has to be injected on a
// second, stream-copy pass over an already-encoded plain clip (see
// docs/decisions.md).
func generateRotatedFixture(t *testing.T, dir string) string {
	t.Helper()

	plain := generateFixture(t, dir, "plain.mp4",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:duration=2:rate=30", "-c:v", "libx264")

	rotated := filepath.Join(dir, "rotated.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg",
		"-y", "-v", "error", "-display_rotation:v:0", "90", "-i", plain, "-c", "copy", rotated)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not inject rotation metadata: %v\n%s", err, out)
	}

	return rotated
}

// validWAVBytes builds one minimal, valid canonical-layout PCM WAV
// file's own bytes - RIFF/WAVE, a 16-byte fmt chunk, then one data
// chunk of frames all-zero frames - at channels and sampleRate, for
// a fakeRunner to write as a profile's own sandboxed output in a
// test that needs Audio's or Sound's own rebuild step to actually
// accept what it finds there.
func validWAVBytes(channels, sampleRate, frames int) []byte {
	data := make([]byte, frames*channels*2)

	fmtBody := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtBody[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(fmtBody[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(fmtBody[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(fmtBody[8:12], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(fmtBody[12:14], uint16(channels*2))
	binary.LittleEndian.PutUint16(fmtBody[14:16], 16)

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	writeUint32LE(&buf, uint32(4+8+len(fmtBody)+8+len(data)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	writeUint32LE(&buf, uint32(len(fmtBody)))
	buf.Write(fmtBody)
	buf.WriteString("data")
	writeUint32LE(&buf, uint32(len(data)))
	buf.Write(data)
	return buf.Bytes()
}

// writeUint32LE appends v to buf as 4 little-endian bytes.
func writeUint32LE(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// validMP4SPSHex/validMP4PPSHex are one real AVC Main profile,
// level 3.1, 320x240 SPS/PPS pair, generated once with the host's
// own ffmpeg (never through this package) and hardcoded here so
// validMP4Bytes needs no ffmpeg at all.
const (
	validMP4SPSHex = "674d401feca0a0fd8088000003000800000300a078c18cb0"
	validMP4PPSHex = "68ebe3cb20"
)

// nalSample builds one length-prefixed sample from the given NAL
// unit types: remuxMP4 never decodes slice data, only reads a NAL's
// own type byte, so a few arbitrary payload bytes are enough.
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

// validMP4Bytes builds a minimal MP4 with mp4ff directly - never
// through this package's own production code - that Video's and
// Animation's own rebuild step will accept: one avc1 video track
// (AVC Main profile, 320x240, an IDR sample followed by two non-IDR
// ones) and, when withAudio, one mp4a AAC-LC audio track too. It is
// for a fakeRunner to write as a profile's own sandboxed output in a
// test that needs the rebuild step to actually succeed.
func validMP4Bytes(t *testing.T, withAudio bool) []byte {
	t.Helper()

	videoSamples := [][]byte{nalSample(5), nalSample(1), nalSample(1)}
	video := mp4.CreateEmptyTrak(1, 10000, "video", "und")
	sps, err := hex.DecodeString(validMP4SPSHex)
	if err != nil {
		t.Fatalf("decoding fixture SPS: %v", err)
	}
	pps, err := hex.DecodeString(validMP4PPSHex)
	if err != nil {
		t.Fatalf("decoding fixture PPS: %v", err)
	}
	if err := video.SetAVCDescriptor("avc1", [][]byte{sps}, [][]byte{pps}, true); err != nil {
		t.Fatalf("SetAVCDescriptor: %v", err)
	}
	fillValidSampleTables(t, video, videoSamples)

	traks := []*mp4.TrakBox{video}
	allSamples := [][][]byte{videoSamples}
	if withAudio {
		audioSamples := [][]byte{{0xaa, 0xbb}, {0xaa, 0xbb}, {0xaa, 0xbb}}
		audio := mp4.CreateEmptyTrak(2, 48000, "audio", "und")
		if err := audio.SetAACDescriptorWithChannelConfig(aac.AAClc, 2, 48000); err != nil {
			t.Fatalf("SetAACDescriptorWithChannelConfig: %v", err)
		}
		fillValidSampleTables(t, audio, audioSamples)
		traks = append(traks, audio)
		allSamples = append(allSamples, audioSamples)
	}

	return encodeTestMP4(t, traks, allSamples)
}

// fillValidSampleTables builds trak's own stts/stsc/stsz/stco
// directly from samples, 1000 ticks each, with a placeholder chunk
// offset that encodeTestMP4 fills in once it knows the file's own
// layout.
func fillValidSampleTables(t *testing.T, trak *mp4.TrakBox, samples [][]byte) {
	t.Helper()
	stbl := trak.Mdia.Minf.Stbl
	stts := &mp4.SttsBox{}
	sizes := make([]uint32, len(samples))
	for i, s := range samples {
		stts.SampleCount = append(stts.SampleCount, 1)
		stts.SampleTimeDelta = append(stts.SampleTimeDelta, 1000)
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

// encodeTestMP4 assembles ftyp+moov+mdat from traks and their own
// samples, one chunk per track in the same order as traks, and
// returns the whole file's own bytes.
func encodeTestMP4(t *testing.T, traks []*mp4.TrakBox, allSamples [][][]byte) []byte {
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
		for _, s := range allSamples[i] {
			offset += int64(len(s))
			payload += int64(len(s))
		}
	}

	var buf bytes.Buffer
	if err := ftyp.Encode(&buf); err != nil {
		t.Fatalf("encoding ftyp: %v", err)
	}
	if err := moov.Encode(&buf); err != nil {
		t.Fatalf("encoding moov: %v", err)
	}
	if err := mp4.EncodeHeaderWithSize("mdat", uint64(payload)+8, false, &buf); err != nil {
		t.Fatalf("encoding mdat header: %v", err)
	}
	for _, samples := range allSamples {
		for _, s := range samples {
			buf.Write(s)
		}
	}
	return buf.Bytes()
}

// assertCleanContainer fails t unless path has exactly one h264
// Main-profile video stream, exactly one aac audio stream when
// wantAudio (none otherwise), no other stream of any kind, and no
// format-level metadata tag at all - confirming a rebuilt file is
// exactly the minimal container remuxMP4 is meant to write, not
// merely that ffprobe can read something out of it.
func assertCleanContainer(t *testing.T, path string, wantAudio bool) {
	t.Helper()

	if codec := probeEntry(t, path, "v", "codec_name"); codec != "h264" {
		t.Errorf("video codec = %q, want h264", codec)
	}
	if profile := probeEntry(t, path, "v", "profile"); profile != "Main" {
		t.Errorf("video profile = %q, want Main", profile)
	}

	wantStreams := 1
	if wantAudio {
		wantStreams++
		if codec := probeEntry(t, path, "a", "codec_name"); codec != "aac" {
			t.Errorf("audio codec = %q, want aac", codec)
		}
	} else if codec := probeEntry(t, path, "a", "codec_name"); codec != "" {
		t.Errorf("audio codec = %q, want no audio stream at all", codec)
	}

	if got := probeStreamCount(t, path); got != wantStreams {
		t.Errorf("stream count = %d, want %d", got, wantStreams)
	}
	if tags := probeFormatTags(t, path); tags != "" {
		t.Errorf("format carries metadata %q, want none", tags)
	}
}

// probeStreamCount runs the host's own ffprobe and returns how many
// streams of any kind path declares.
func probeStreamCount(t *testing.T, path string) int {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH")
	}
	cmd := exec.CommandContext(t.Context(), ffprobePath,
		"-v", "error", "-show_entries", "stream=index", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	return len(lines)
}

// formatBrandTags are the format_tags entries ffprobe always
// surfaces for any MP4 at all, straight from its mandatory ftyp box
// (major_brand, minor_version, compatible_brands): not metadata a
// sender or an encoder wrote, and no amount of stripping could ever
// make them disappear, so probeFormatTags excludes them.
var formatBrandTags = []string{"TAG:major_brand=", "TAG:minor_version=", "TAG:compatible_brands="}

// probeFormatTags runs the host's own ffprobe and returns whatever
// format-level metadata tag path carries beyond its own mandatory
// ftyp brand fields (see formatBrandTags), trimmed; empty means
// none - a title, an encoder string, a comment or the like.
func probeFormatTags(t *testing.T, path string) string {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH")
	}
	cmd := exec.CommandContext(t.Context(), ffprobePath,
		"-v", "error", "-show_entries", "format_tags", "-of", "default=noprint_wrappers=1", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}

	var extra []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		isBrandTag := false
		for _, brand := range formatBrandTags {
			if strings.HasPrefix(line, brand) {
				isBrandTag = true
				break
			}
		}
		if !isBrandTag {
			extra = append(extra, line)
		}
	}
	return strings.Join(extra, "\n")
}

// probeEntry runs the host's own ffprobe and returns the trimmed
// value of one stream entry for the first stream of kind - "v" or
// "a" - in path.
func probeEntry(t *testing.T, path, kind, entry string) string {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found on PATH")
	}

	cmd := exec.CommandContext(t.Context(), ffprobePath,
		"-v", "error",
		"-select_streams", kind+":0",
		"-show_entries", "stream="+entry,
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}

	return strings.TrimSpace(string(out))
}
