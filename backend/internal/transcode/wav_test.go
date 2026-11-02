package transcode

// wav_test.go tests rebuildWAV directly, as an internal test: it
// needs to build malformed WAV headers byte by byte, which the
// public Audio/Sound API has no way to ask the sandboxed ffmpeg to
// produce on purpose.

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestWAV builds a minimal WAV file at dir/name from chunks, in
// order, and returns its path. Each chunk already includes its own
// 8-byte id+size header and any padding byte; callers use
// wavChunk/wavFmtChunk/wavDataChunk to build one.
func writeTestWAV(t *testing.T, dir, name string, chunks ...[]byte) string {
	t.Helper()

	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}

	riff := make([]byte, 12)
	copy(riff[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riff[4:8], uint32(4+len(body)))
	copy(riff[8:12], "WAVE")

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, append(riff, body...), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// wavChunk builds one chunk: its own 4-byte id, a little-endian
// 4-byte size and data, padded with one zero byte when data's own
// length is odd.
func wavChunk(id string, data []byte) []byte {
	chunk := make([]byte, 8, 8+len(data)+1)
	copy(chunk[0:4], id)
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(len(data)))
	chunk = append(chunk, data...)
	if len(data)%2 == 1 {
		chunk = append(chunk, 0)
	}
	return chunk
}

// wavFmtChunk builds a 16-byte canonical PCM fmt chunk.
func wavFmtChunk(channels, bits uint16, sampleRate uint32) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint16(body[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(body[2:4], channels)
	binary.LittleEndian.PutUint32(body[4:8], sampleRate)
	binary.LittleEndian.PutUint32(body[8:12], sampleRate*uint32(channels)*uint32(bits)/8)
	binary.LittleEndian.PutUint16(body[12:14], channels*bits/8)
	binary.LittleEndian.PutUint16(body[14:16], bits)
	return wavChunk("fmt ", body)
}

// wavDataChunk builds a data chunk holding n bytes of arbitrary
// sample data.
func wavDataChunk(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i)
	}
	return wavChunk("data", data)
}

func TestRebuildWAV(t *testing.T) {
	t.Parallel()

	const rate = 48000

	tests := map[string]struct {
		chunks    [][]byte
		wantErr   error
		wantChans uint16
		wantLen   int
	}{
		"mono": {
			chunks:    [][]byte{wavFmtChunk(1, 16, rate), wavDataChunk(400)},
			wantChans: 1,
			wantLen:   400,
		},
		"stereo": {
			chunks:    [][]byte{wavFmtChunk(2, 16, rate), wavDataChunk(800)},
			wantChans: 2,
			wantLen:   800,
		},
		"extra fmt bytes (cbSize) are read and discarded": {
			chunks:    [][]byte{wavChunk("fmt ", append(wavFmtChunk(1, 16, rate)[8:], 0, 0)), wavDataChunk(4)},
			wantChans: 1,
			wantLen:   4,
		},
		"skippable LIST chunk before data": {
			chunks:    [][]byte{wavFmtChunk(1, 16, rate), wavChunk("LIST", []byte("INFOxxxx")), wavDataChunk(4)},
			wantChans: 1,
			wantLen:   4,
		},
		"skippable fact chunk before data": {
			chunks:    [][]byte{wavFmtChunk(2, 16, rate), wavChunk("fact", []byte{1, 0, 0, 0}), wavDataChunk(8)},
			wantChans: 2,
			wantLen:   8,
		},
		"wrong format tag": {
			chunks:  [][]byte{wavChunk("fmt ", func() []byte { b := wavFmtChunk(1, 16, rate)[8:]; b[0] = 3; return b }()), wavDataChunk(4)},
			wantErr: ErrFailed,
		},
		"wrong sample rate": {
			chunks:  [][]byte{wavFmtChunk(1, 16, rate+1), wavDataChunk(4)},
			wantErr: ErrFailed,
		},
		"wrong bit depth": {
			chunks:  [][]byte{wavFmtChunk(1, 8, rate), wavDataChunk(4)},
			wantErr: ErrFailed,
		},
		"zero channels": {
			chunks:  [][]byte{wavFmtChunk(0, 16, rate), wavDataChunk(4)},
			wantErr: ErrFailed,
		},
		"three channels": {
			chunks:  [][]byte{wavFmtChunk(3, 16, rate), wavDataChunk(6)},
			wantErr: ErrFailed,
		},
		"data before fmt": {
			chunks:  [][]byte{wavDataChunk(4), wavFmtChunk(1, 16, rate)},
			wantErr: ErrFailed,
		},
		"no data chunk at all": {
			chunks:  [][]byte{wavFmtChunk(1, 16, rate)},
			wantErr: ErrFailed,
		},
		"too many skippable chunks": {
			chunks: [][]byte{
				wavFmtChunk(1, 16, rate),
				wavChunk("LIST", []byte{0}), wavChunk("LIST", []byte{0}),
				wavChunk("LIST", []byte{0}), wavChunk("LIST", []byte{0}),
				wavChunk("LIST", []byte{0}),
				wavDataChunk(4),
			},
			wantErr: ErrFailed,
		},
		"unrecognised chunk is refused, not skipped": {
			chunks:  [][]byte{wavFmtChunk(1, 16, rate), wavChunk("JUNK", []byte{0, 0, 0, 0}), wavDataChunk(4)},
			wantErr: ErrFailed,
		},
		"odd data length is not a whole number of frames": {
			chunks:  [][]byte{wavFmtChunk(1, 16, rate), wavChunk("data", make([]byte, 5))},
			wantErr: ErrFailed,
		},
		"data length is not a whole number of stereo frames": {
			chunks:  [][]byte{wavFmtChunk(2, 16, rate), wavChunk("data", make([]byte, 6))},
			wantErr: ErrFailed,
		},
		"empty data chunk": {
			chunks:  [][]byte{wavFmtChunk(1, 16, rate), wavChunk("data", nil)},
			wantErr: ErrFailed,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			in := writeTestWAV(t, dir, "in.wav", tt.chunks...)
			out := filepath.Join(dir, "out.wav")

			err := rebuildWAV(in, out, rate, 1<<20)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("rebuildWAV() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("rebuildWAV() error = %v, want nil", err)
			}

			gotFormat, gotData := readRebuiltWAV(t, out)
			if gotFormat.channels != tt.wantChans {
				t.Errorf("channels = %d, want %d", gotFormat.channels, tt.wantChans)
			}
			if gotFormat.sampleRate != rate {
				t.Errorf("sample rate = %d, want %d", gotFormat.sampleRate, rate)
			}
			if len(gotData) != tt.wantLen {
				t.Errorf("data length = %d, want %d", len(gotData), tt.wantLen)
			}
		})
	}
}

func TestRebuildWAV_DataLengthPastFileEnd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A data chunk that declares far more than the file actually
	// holds: a crafted length field, not a truncated read.
	chunk := wavChunk("data", make([]byte, 8))
	binary.LittleEndian.PutUint32(chunk[4:8], 1<<20)
	in := writeTestWAV(t, dir, "in.wav", wavFmtChunk(1, 16, 48000), chunk)

	err := rebuildWAV(in, filepath.Join(dir, "out.wav"), 48000, 1<<20)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("rebuildWAV() error = %v, want %v", err, ErrFailed)
	}
}

func TestRebuildWAV_Truncated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.wav")
	// Just the RIFF/WAVE magic, nothing else: readWAVHeader must fail
	// cleanly on a short file rather than panic or read past it.
	if err := os.WriteFile(in, []byte("RIFF\x00\x00\x00\x00WAVE"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := rebuildWAV(in, filepath.Join(dir, "out.wav"), 48000, 1<<20)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("rebuildWAV() error = %v, want %v", err, ErrFailed)
	}
}

func TestRebuildWAV_NotRIFF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.wav")
	if err := os.WriteFile(in, []byte("not a wave file at all, just text"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := rebuildWAV(in, filepath.Join(dir, "out.wav"), 48000, 1<<20)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("rebuildWAV() error = %v, want %v", err, ErrFailed)
	}
}

func TestRebuildWAV_OverMaxOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := writeTestWAV(t, dir, "in.wav", wavFmtChunk(2, 16, 48000), wavDataChunk(100))

	err := rebuildWAV(in, filepath.Join(dir, "out.wav"), 48000, wavHeaderSize+10)
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("rebuildWAV() error = %v, want %v", err, ErrTooLong)
	}
}

func TestWriteWAV_CopyError(t *testing.T) {
	t.Parallel()

	// data has fewer bytes than dataLen claims: io.CopyN must then
	// report a short read rather than writing a truncated file
	// silently.
	out := filepath.Join(t.TempDir(), "out.wav")
	err := writeWAV(out, pcmFormat{channels: 1, sampleRate: 48000}, strings.NewReader("short"), 100)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("writeWAV() error = %v, want %v", err, ErrFailed)
	}
}

func TestWriteWAV_AlreadyExists(t *testing.T) {
	t.Parallel()

	// O_EXCL refuses to create a file that is already there, the
	// same guard rebuildWAV relies on to never silently overwrite
	// whatever the caller already left at outPath.
	out := filepath.Join(t.TempDir(), "out.wav")
	if err := os.WriteFile(out, []byte("already here"), 0o600); err != nil {
		t.Fatalf("pre-creating output: %v", err)
	}

	err := writeWAV(out, pcmFormat{channels: 1, sampleRate: 48000}, strings.NewReader("data"), 4)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("writeWAV() error = %v, want %v", err, ErrFailed)
	}
}

func TestSkipChunk_Truncated(t *testing.T) {
	t.Parallel()

	err := skipChunk(strings.NewReader("short"), 100)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("skipChunk() error = %v, want %v", err, ErrFailed)
	}
}

func TestSkipChunk_PadTruncated(t *testing.T) {
	t.Parallel()

	// An odd-sized chunk whose body reads cleanly but whose single
	// pad byte is then missing: skipChunk's own call to skipPad must
	// still report the truncation.
	err := skipChunk(strings.NewReader("odd"), 3)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("skipChunk() error = %v, want %v", err, ErrFailed)
	}
}

func TestRebuildWAV_StatError(t *testing.T) {
	t.Parallel()

	// rebuildWAV's own call to Stat happens on an already-open file,
	// which practically never fails; its error path is instead
	// confirmed by calling checkWAVData directly with a Seek that
	// fails.
	err := checkWAVData(failingSeeker{}, 100, pcmFormat{channels: 1, sampleRate: 48000}, 4, 1<<20)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("checkWAVData() error = %v, want %v", err, ErrFailed)
	}
}

// failingSeeker's Seek always fails, for exercising checkWAVData's
// own error handling around finding the current file offset.
type failingSeeker struct{}

func (failingSeeker) Seek(int64, int) (int64, error) { return 0, io.ErrClosedPipe }

// readRebuiltWAV reads back a file rebuildWAV wrote and confirms it
// is exactly a canonical 44-byte-header PCM WAV: wavHeaderSize bytes
// of header, immediately followed by data and nothing else.
func readRebuiltWAV(t *testing.T, path string) (pcmFormat, []byte) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading rebuilt file: %v", err)
	}
	if len(raw) < wavHeaderSize {
		t.Fatalf("rebuilt file is %d bytes, shorter than the canonical header", len(raw))
	}
	if string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" || string(raw[12:16]) != "fmt " || string(raw[36:40]) != "data" {
		t.Fatalf("rebuilt file does not have the canonical RIFF/WAVE/fmt/data layout: %q", raw[:44])
	}

	format := pcmFormat{
		channels:   binary.LittleEndian.Uint16(raw[22:24]),
		sampleRate: binary.LittleEndian.Uint32(raw[24:28]),
	}
	dataLen := binary.LittleEndian.Uint32(raw[40:44])
	if int(dataLen) != len(raw)-wavHeaderSize {
		t.Fatalf("declared data length %d does not match the %d actual bytes following the header", dataLen, len(raw)-wavHeaderSize)
	}
	return format, raw[wavHeaderSize:]
}
