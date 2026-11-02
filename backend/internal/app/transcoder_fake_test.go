package app_test

// transcoder_fake_test.go holds fakeTranscoder, the stand-in for the
// sandboxed transcoder every fixture wires in, and onePlayableMP4,
// the realistic video fixture play-copy tests feed it: split out of
// helpers_test.go, which was already near the file-length gate, the
// same reasoning connector_fakes_test.go gives for its own split.

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
)

// avcDecoderConfig640x480 is an AVCDecoderConfigurationRecord whose
// one SPS decodes to exactly 640x480, generated from the same
// encoding backend/internal/connector/helpers_test.go once used for
// its own, now-deleted copy of this fixture.
var avcDecoderConfig640x480 = []byte{0x01, 0x42, 0x00, 0x1E, 0xFF, 0x01, 0x00, 0x08, 0x67, 0x42, 0x00, 0x1E, 0xF8, 0x14, 0x07, 0xA0, 0x00}

// onePlayableMP4 returns a minimal, well-formed MP4 file - an
// ordinary H.264 video at 640x480 - for a test that needs bytes that
// actually sniff and attach as a video rather than an arbitrary
// placeholder: a leading ftyp box (the one box
// http.DetectContentType's own mp4 signature actually keys on)
// followed by one moov box with an mvhd and a single video trak,
// just enough of a real file's own box structure to pass as one.
func onePlayableMP4() []byte {
	mediaHeader := func(timescale, duration uint32) []byte {
		buf := make([]byte, 20)
		binary.BigEndian.PutUint32(buf[12:16], timescale)
		binary.BigEndian.PutUint32(buf[16:20], duration)
		return buf
	}
	box := func(boxType string, body []byte) []byte {
		buf := make([]byte, 8+len(body))
		binary.BigEndian.PutUint32(buf[0:4], uint32(len(buf)))
		copy(buf[4:8], boxType)
		copy(buf[8:], body)
		return buf
	}

	entryBody := append(make([]byte, 78), box("avcC", avcDecoderConfig640x480)...)
	binary.BigEndian.PutUint16(entryBody[24:26], 640)
	binary.BigEndian.PutUint16(entryBody[26:28], 480)

	sampleDescription := make([]byte, 8)
	binary.BigEndian.PutUint32(sampleDescription[4:8], 1) // entry_count
	sampleDescription = append(sampleDescription, box("avc1", entryBody)...)

	mdhd := box("mdhd", mediaHeader(1000, 5000))
	hdlr := box("hdlr", append(make([]byte, 8), []byte("vide")...))
	stsd := box("stsd", sampleDescription)
	stsz := box("stsz", make([]byte, 12))
	stbl := box("stbl", append(append([]byte{}, stsd...), stsz...))
	minf := box("minf", stbl)
	mdia := box("mdia", append(append(append([]byte{}, mdhd...), hdlr...), minf...))
	trak := box("trak", mdia)
	mvhd := box("mvhd", mediaHeader(1000, 5000))
	moov := box("moov", append(append([]byte{}, mvhd...), trak...))
	ftyp := box("ftyp", append([]byte("mp4 "), make([]byte, 4)...)) // major brand "mp4 ", zero minor version

	return append(ftyp, moov...)
}

// fakeTranscoder stands in for the sandboxed transcoder: each profile
// method copies in's bytes to a fresh "out<ext>" file of its own under
// a job directory inside outDir, mirroring the real contract (see
// transcode's own package doc comment) closely enough that code
// moving or removing that job directory is exercised the same way it
// would be against the real one. err, when set, makes every call fail
// instead, the same as ffmpeg or the sandbox being unavailable.
type fakeTranscoder struct {
	mu    sync.Mutex
	calls []string // one entry per call, the profile name
	err   error
}

func (t *fakeTranscoder) Video(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, "video", in, outDir, ".mp4")
}

func (t *fakeTranscoder) Animation(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, "animation", in, outDir, ".mp4")
}

func (t *fakeTranscoder) Audio(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, "audio", in, outDir, ".ogg")
}

func (t *fakeTranscoder) Sound(ctx context.Context, in, outDir string) (string, error) {
	return t.run(ctx, "sound", in, outDir, ".wav")
}

// StillFrame records the call like every other profile, but writes a
// real, tiny, decodable PNG regardless of in's own bytes, never a
// copy of them: unlike Video, Animation, Audio and Sound, which only
// need to prove the right profile ran, a still frame's whole point is
// producing bytes Go's own decoders can read where the original
// could not (see app.reencodeForDisplay), which a plain copy-through
// could never simulate.
func (t *fakeTranscoder) StillFrame(ctx context.Context, in, outDir string) (string, error) {
	t.mu.Lock()
	t.calls = append(t.calls, "stillframe")
	err := t.err
	t.mu.Unlock()

	if err != nil {
		return "", err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if _, statErr := os.Stat(in); statErr != nil {
		return "", statErr
	}

	jobDir, mkErr := os.MkdirTemp(outDir, "faketranscode-*")
	if mkErr != nil {
		return "", mkErr
	}

	out := filepath.Join(jobDir, "out.png")

	return out, os.WriteFile(out, onePixelPNGFrame(), 0o600)
}

// onePixelPNGFrame returns a tiny, real, decodable PNG, for
// fakeTranscoder.StillFrame to write regardless of its own input.
func onePixelPNGFrame() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1)))

	return buf.Bytes()
}

// run records name as a call and, unless err is set, copies in's bytes
// into a fresh job directory under outDir, as "out"+ext.
func (t *fakeTranscoder) run(ctx context.Context, name, in, outDir, ext string) (string, error) {
	t.mu.Lock()
	t.calls = append(t.calls, name)
	err := t.err
	t.mu.Unlock()

	if err != nil {
		return "", err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}

	data, readErr := os.ReadFile(in)
	if readErr != nil {
		return "", readErr
	}

	jobDir, mkErr := os.MkdirTemp(outDir, "faketranscode-*")
	if mkErr != nil {
		return "", mkErr
	}

	out := filepath.Join(jobDir, "out"+ext)
	if writeErr := os.WriteFile(out, data, 0o600); writeErr != nil {
		return "", writeErr
	}

	return out, nil
}

// callCount returns how many calls fakeTranscoder has recorded so far,
// across every profile.
func (t *fakeTranscoder) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.calls)
}

// lastCall returns the profile name of the most recent call, or "" if
// there have been none yet.
func (t *fakeTranscoder) lastCall() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.calls) == 0 {
		return ""
	}

	return t.calls[len(t.calls)-1]
}
