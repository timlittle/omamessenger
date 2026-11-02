package transcode

// remux_test.go tests remux.go directly, as an internal test: an
// adversarial MP4 built with mp4ff by hand (see
// internal_helpers_test.go) is the only way to reach most of these
// cases at all, and going through the full Transcoder for each one
// would exercise a lot of sandbox machinery (slot acquisition,
// checkOutput) that has nothing to do with what is under test here.
// checkSPS, checkPPS and checkAudioSpecificConfig are tested
// directly against hand-built values, in avcvalidate_test.go, for
// the same reason.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// rawBox builds one top-level box's own raw bytes by hand: an 8-byte
// header (declaredSize, boxType) followed by bodyLen filler bytes.
// declaredSize and the body's own actual length are independent, so
// a test can claim a size the body does not really back up.
func rawBox(boxType string, declaredSize uint32, bodyLen int) []byte {
	buf := make([]byte, 8+bodyLen)
	binary.BigEndian.PutUint32(buf[0:4], declaredSize)
	copy(buf[4:8], boxType)
	return buf
}

// writeRawFile concatenates boxes in order and writes them to
// dir/name, returning its path.
func writeRawFile(t *testing.T, dir, name string, boxes ...[]byte) string {
	t.Helper()
	var all []byte
	for _, b := range boxes {
		all = append(all, b...)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, all, 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestCheckTopLevelBoxes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		boxes   func() [][]byte
		wantErr bool
	}{
		"ordinary ftyp/moov/mdat": {boxes: func() [][]byte {
			return [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", 16, 8), rawBox("mdat", 8, 0)}
		}},
		"oversized moov": {boxes: func() [][]byte {
			return [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", maxMoovBytes+100, maxMoovBytes+100-8), rawBox("mdat", 8, 0)}
		}, wantErr: true},
		"moov at exactly the cap is fine": {boxes: func() [][]byte {
			return [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", maxMoovBytes, maxMoovBytes-8), rawBox("mdat", 8, 0)}
		}},
		"two moov boxes": {boxes: func() [][]byte {
			return [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", 16, 8), rawBox("moov", 16, 8), rawBox("mdat", 8, 0)}
		}, wantErr: true},
		"too many top-level boxes": {boxes: func() [][]byte {
			boxes := [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", 16, 8)}
			for i := 0; i < maxTopLevelBoxes; i++ {
				boxes = append(boxes, rawBox("free", 8, 0))
			}
			return append(boxes, rawBox("mdat", 8, 0))
		}, wantErr: true},
		"size runs past the file's own end": {boxes: func() [][]byte {
			// Declares 1000 bytes but the file holds only this one
			// small box: nothing backs up the other 992 bytes.
			return [][]byte{rawBox("ftyp", 1000, 0)}
		}, wantErr: true},
		"second box's size runs past the end": {boxes: func() [][]byte {
			return [][]byte{rawBox("ftyp", 8, 0), rawBox("moov", 1000, 8)}
		}, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeRawFile(t, t.TempDir(), "in.mp4", tt.boxes()...)
			f := mustOpen(t, path)
			defer func() { _ = f.Close() }()

			err := checkTopLevelBoxes(f)
			if tt.wantErr && !errors.Is(err, ErrFailed) {
				t.Fatalf("checkTopLevelBoxes() error = %v, want %v", err, ErrFailed)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkTopLevelBoxes() error = %v, want nil", err)
			}
		})
	}
}

// seekFailAt wraps a real io.ReadSeeker but fails the call numbered
// failAfter+1 and every one after it, for exercising a caller's own
// handling of a Seek failure a real file essentially never produces.
type seekFailAt struct {
	io.ReadSeeker
	failAfter int
	calls     int
}

func (s *seekFailAt) Seek(offset int64, whence int) (int64, error) {
	s.calls++
	if s.calls > s.failAfter {
		return 0, errors.New("seekFailAt: injected failure")
	}
	return s.ReadSeeker.Seek(offset, whence)
}

func TestCheckTopLevelBoxes_SeekErrors(t *testing.T) {
	t.Parallel()

	path := writeRawFile(t, t.TempDir(), "in.mp4", rawBox("ftyp", 8, 0), rawBox("moov", 16, 8), rawBox("mdat", 8, 0))

	tests := map[string]int{
		"finding file size fails":     0,
		"rewinding to start fails":    1,
		"skipping a box's body fails": 2,
	}
	for name, failAfter := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := mustOpen(t, path)
			defer func() { _ = f.Close() }()

			err := checkTopLevelBoxes(&seekFailAt{ReadSeeker: f, failAfter: failAfter})
			if !errors.Is(err, ErrFailed) {
				t.Fatalf("checkTopLevelBoxes() error = %v, want %v", err, ErrFailed)
			}
		})
	}
}

func TestRemuxMP4_Refusals(t *testing.T) {
	t.Parallel()

	runRemuxRefusals(t, map[string]remuxRefusal{
		"no video track": {build: func(t *testing.T, dir string) string {
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{audio}, [][][]byte{defaultAudioSamples()})
		}},
		"two video tracks": {build: func(t *testing.T, dir string) string {
			v1 := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			v2 := mp4.CreateEmptyTrak(2, 10000, "video", "und")
			if err := v2.SetAVCDescriptor("avc1", [][]byte{mustHex(t, mainSPSHex)}, [][]byte{mustHex(t, mainPPSHex)}, true); err != nil {
				t.Fatalf("SetAVCDescriptor: %v", err)
			}
			fillTestSampleTables(t, v2, defaultVideoSamples(), []uint32{1000, 1000, 1000})
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{v1, v2}, [][][]byte{defaultVideoSamples(), defaultVideoSamples()})
		}},
		"audio not allowed for this profile": {
			build: func(t *testing.T, dir string) string {
				video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
				audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
				return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
			},
			caps: func() *remuxCaps { c := defaultCaps(); c.allowAudio = false; return &c }(),
		},
		"two sample entries in stsd": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			second := mp4.CreateVisualSampleEntryBox("avc1", 320, 240, video.Mdia.Minf.Stbl.Stsd.AvcX.AvcC)
			video.Mdia.Minf.Stbl.Stsd.AddChild(second)
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"zero SPS": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			video.Mdia.Minf.Stbl.Stsd.AvcX.AvcC.SPSnalus = nil
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"two SPS": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			avcC := video.Mdia.Minf.Stbl.Stsd.AvcX.AvcC
			avcC.SPSnalus = append(avcC.SPSnalus, avcC.SPSnalus[0])
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"zero PPS": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			video.Mdia.Minf.Stbl.Stsd.AvcX.AvcC.PPSnalus = nil
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"two PPS": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			avcC := video.Mdia.Minf.Stbl.Stsd.AvcX.AvcC
			avcC.PPSnalus = append(avcC.PPSnalus, avcC.PPSnalus[0])
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"SPS profile is not Main": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, highProfileSPSHex, highProfilePPSHex, defaultVideoSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"SPS level over 4.0": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, level51SPSHex, mainPPSHex, defaultVideoSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"SPS dimensions over 1280": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, largeSPSHex, mainPPSHex, defaultVideoSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"SPS size does not match the sample entry": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			video.Mdia.Minf.Stbl.Stsd.AvcX.Width = 640
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})
		}},
		"mp4a has no esds": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
			mp4a := audio.Mdia.Minf.Stbl.Stsd.Mp4a
			mp4a.Esds = nil
			mp4a.Children = nil // Encode writes Children, not the named Esds field, so both must be cleared
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"AAC object type is not AAC-LC": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.HEAACv1, 48000, 2, defaultAudioSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"AAC sample rate not 44.1 or 48 kHz": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 32000, 2, defaultAudioSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"AAC channel configuration is not 1 or 2": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 6, defaultAudioSamples())
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"AAC sample rate does not match the sample entry": {build: func(t *testing.T, dir string) string {
			video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
			audio := buildAudioTrakForTest(t, aac.AAClc, 48000, 2, defaultAudioSamples())
			audio.Mdia.Minf.Stbl.Stsd.Mp4a.SampleRate = 44100
			return writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
		}},
		"no moov box": {build: func(t *testing.T, dir string) string {
			path := filepath.Join(dir, "in.mp4")
			ftyp := mp4.NewFtyp(mp4.BrandIsom, 0, nil)
			var buf bytes.Buffer
			if err := ftyp.Encode(&buf); err != nil {
				t.Fatalf("encoding ftyp: %v", err)
			}
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatalf("writing fixture: %v", err)
			}
			return path
		}},
		"intermediate does not exist": {build: func(_ *testing.T, dir string) string {
			return filepath.Join(dir, "missing.mp4")
		}},
	})
}

func TestRemuxMP4_WrongNALLengthSize(t *testing.T) {
	t.Parallel()

	// DecodeAVCDecConfRec itself refuses any avcC whose NAL length
	// size is not 4 while parsing the file at all (see
	// avc.ErrLengthSize), before remuxMP4's own validation ever
	// runs: patching a valid file's own avcC length-size byte
	// confirms the file fails to parse, rather than silently
	// succeeding with the wrong length size.
	dir := t.TempDir()
	video := buildVideoTrakForTest(t, mainSPSHex, mainPPSHex, defaultVideoSamples())
	in := writeTestMP4File(t, dir, "in.mp4", []*mp4.TrakBox{video}, [][][]byte{defaultVideoSamples()})

	raw, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	idx := bytes.Index(raw, []byte("avcC"))
	if idx < 0 {
		t.Fatal("fixture has no avcC box")
	}
	// Byte 4 of the AVCDecoderConfigurationRecord, right after the
	// box header, holds lengthSizeMinusOne in its low two bits: 0xff
	// means length size 4 (see avc.EncodeSW); clearing those bits
	// claims length size 1 instead.
	raw[idx+4+4] &^= 0x03
	if err := os.WriteFile(in, raw, 0o600); err != nil {
		t.Fatalf("writing patched fixture: %v", err)
	}

	err = remuxMP4(in, filepath.Join(dir, "out.mp4"), defaultCaps())
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("remuxMP4() error = %v, want %v", err, ErrFailed)
	}
}

func TestRemuxMP4_TruncatedBoxDeclaringAnImpossibleSize(t *testing.T) {
	t.Parallel()

	// A moov box declaring a size larger than what actually follows
	// it is exactly the kind of input remuxMP4's own recover exists
	// for: whether mp4ff's own decoder turns this into a clean error
	// or, someday, something that panics instead, the outcome this
	// package promises its own caller is the same either way.
	dir := t.TempDir()
	path := filepath.Join(dir, "in.mp4")
	var buf bytes.Buffer
	if _, err := buf.WriteString("ftypisom"); err != nil {
		t.Fatalf("writing ftyp: %v", err)
	}
	buf.Write([]byte{0, 0, 0, 100})
	buf.WriteString("moov")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := remuxMP4(path, filepath.Join(dir, "out.mp4"), defaultCaps())
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("remuxMP4() error = %v, want %v", err, ErrFailed)
	}
}

// FuzzRemux feeds arbitrary bytes to remuxMP4 as the "sandboxed
// ffmpeg output" and checks two invariants that must hold for any
// input at all: remuxMP4 never panics (recovered itself turns into
// ErrFailed, but a crash anywhere else under it would still fail the
// fuzzer), and whenever it does succeed, feeding its own rebuilt
// output back through remuxMP4 a second time succeeds too - this
// package's own output must always already satisfy its own
// validator.
func FuzzRemux(f *testing.F) {
	dir := f.TempDir()
	video := buildVideoTrakForTest(f, mainSPSHex, mainPPSHex, defaultVideoSamples())
	audio := buildAudioTrakForTest(f, aac.AAClc, 48000, 2, defaultAudioSamples())
	valid := writeTestMP4File(f, dir, "seed.mp4", []*mp4.TrakBox{video, audio}, [][][]byte{defaultVideoSamples(), defaultAudioSamples()})
	seed, err := os.ReadFile(valid)
	if err != nil {
		f.Fatalf("reading seed fixture: %v", err)
	}
	f.Add(seed)
	f.Add([]byte("not an mp4 at all"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.mp4")
		if err := os.WriteFile(in, data, 0o600); err != nil {
			t.Fatalf("writing fuzz input: %v", err)
		}
		out := filepath.Join(dir, "out.mp4")

		err := remuxMP4(in, out, defaultCaps())
		if err == nil {
			out2 := filepath.Join(dir, "out2.mp4")
			if err := remuxMP4(out, out2, defaultCaps()); err != nil {
				t.Fatalf("remuxMP4() succeeded once but its own output failed a second pass: %v", err)
			}
		}
	})
}
