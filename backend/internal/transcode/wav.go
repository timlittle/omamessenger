package transcode

// wav.go rebuilds the WAV file ffmpeg wrote inside the sandbox into
// a brand-new file this package writes itself: only the handful of
// fields a canonical 16-bit PCM WAV needs are ever read from
// ffmpeg's own output - its own chunk order, any chunk beyond a
// small, named allow-list, and anything at all past the data chunk
// are discarded rather than trusted, so a hijacked ffmpeg can shape
// only the PCM samples themselves, never the container the shell
// actually opens.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// wavHeaderSize is the exact size of the canonical PCM WAV header
// this package writes: RIFF/WAVE (12 bytes), a 16-byte fmt chunk
// with its own 8-byte chunk header, and a data chunk's own 8-byte
// header, with no other chunk in between.
const wavHeaderSize = 44

// bytesPerSample16 is how many bytes one 16-bit PCM sample occupies,
// per channel.
const bytesPerSample16 = 2

// pcmFormatTag is the WAV fmt chunk's own audio format code for
// uncompressed linear PCM.
const pcmFormatTag = 1

// bitsPerSamplePCM16 is the only bit depth this package accepts or
// ever writes.
const bitsPerSamplePCM16 = 16

// wavMaxSkippedChunks bounds how many chunks between fmt and data
// rebuildWAV will skip past looking for data, so a crafted file
// cannot make this package scan forever: ffmpeg's own bitexact WAV
// muxer needs at most one or two.
const wavMaxSkippedChunks = 4

// wavSkippableChunkIDs are the only chunk kinds rebuildWAV will skip
// over on its way to data: both harmless and informational.
// Anything else - a second data chunk, or a chunk this package has
// no reason to ever see from its own fixed ffmpeg invocation - is
// refused outright rather than skipped.
var wavSkippableChunkIDs = map[string]bool{
	"LIST": true,
	"fact": true,
}

// pcmFormat is the handful of fmt-chunk fields rebuildWAV trusts
// once validated: everything a canonical PCM header needs to
// describe the samples that follow.
type pcmFormat struct {
	channels   uint16
	sampleRate uint32
}

// rebuildWAV validates path - a WAV file the sandboxed ffmpeg wrote -
// against wantSampleRate and maxOutput, then writes a new file at
// outPath holding this package's own canonical 44-byte PCM header
// followed by exactly the validated data bytes, copied in bounded
// pieces rather than held in memory at once. It returns ErrFailed,
// wrapped, for anything about path that is not exactly the shape
// this package's own fixed ffmpeg invocation should have produced.
func rebuildWAV(path, outPath string, wantSampleRate int, maxOutput int64) error {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: opening intermediate: %w", ErrFailed, err)
	}
	defer func() {
		_ = in.Close() // read-only handle: nothing left to report that matters once the checks below have already returned their own error
	}()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("%w: statting intermediate: %w", ErrFailed, err)
	}

	format, dataLen, err := readWAVHeader(in, wantSampleRate)
	if err != nil {
		return err
	}
	if err := checkWAVData(in, info.Size(), format, dataLen, maxOutput); err != nil {
		return err
	}

	return writeWAV(outPath, format, in, dataLen)
}

// checkWAVData confirms dataLen is a whole number of frames, non-
// zero, reaches no further than fileSize from f's own current
// position (the first byte of the data chunk's own samples), and
// keeps the rebuilt output within maxOutput.
func checkWAVData(f io.Seeker, fileSize int64, format pcmFormat, dataLen int64, maxOutput int64) error {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("%w: finding data offset: %w", ErrFailed, err)
	}
	if dataLen > fileSize-offset {
		return fmt.Errorf("%w: data chunk reaches past the file's own end", ErrFailed)
	}

	frameSize := int64(format.channels) * bytesPerSample16
	if dataLen == 0 || dataLen%frameSize != 0 {
		return fmt.Errorf("%w: data length %d is not a whole number of frames", ErrFailed, dataLen)
	}
	if wavHeaderSize+dataLen > maxOutput {
		return fmt.Errorf("%w: rebuilt output would be %d bytes", ErrTooLong, wavHeaderSize+dataLen)
	}
	return nil
}

// readWAVHeader reads f's own 12-byte RIFF/WAVE header, then walks
// its chunks (see findDataChunk) to find and validate fmt and locate
// data. f is left positioned at the first byte of the data chunk's
// own samples.
func readWAVHeader(f io.Reader, wantSampleRate int) (pcmFormat, int64, error) {
	if err := readMagic(f, "RIFF"); err != nil {
		return pcmFormat{}, 0, err
	}
	if _, err := readUint32LE(f); err != nil { // the RIFF chunk's own declared size: unused, nothing downstream of this package ever reads it
		return pcmFormat{}, 0, err
	}
	if err := readMagic(f, "WAVE"); err != nil {
		return pcmFormat{}, 0, err
	}
	return findDataChunk(f, wantSampleRate)
}

// wavChunkScanner is findDataChunk's own loop state: the fmt
// chunk's validated fields once found, and how many skippable
// chunks it has already passed.
type wavChunkScanner struct {
	format     pcmFormat
	haveFormat bool
	skipped    int
}

// handle processes one chunk header already read from f. It returns
// (dataLen, true, nil) once data is found, (0, false, nil) to keep
// reading, or a non-nil error for anything findDataChunk must refuse
// or could not read.
func (s *wavChunkScanner) handle(f io.Reader, header wavChunkHeader, wantSampleRate int) (int64, bool, error) {
	switch {
	case header.id == "fmt ":
		format, err := parseFMTChunk(f, header.size, wantSampleRate)
		if err != nil {
			return 0, false, err
		}
		s.format, s.haveFormat = format, true
		return 0, false, nil
	case header.id == "data":
		if !s.haveFormat {
			return 0, false, fmt.Errorf("%w: data chunk before fmt", ErrFailed)
		}
		return header.size, true, nil
	case wavSkippableChunkIDs[header.id] && s.skipped < wavMaxSkippedChunks:
		if err := skipChunk(f, header.size); err != nil {
			return 0, false, err
		}
		s.skipped++
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("%w: unexpected chunk %q", ErrFailed, header.id)
	}
}

// findDataChunk walks f's own chunks in order, immediately after the
// 12-byte RIFF/WAVE header readWAVHeader already consumed, until it
// finds data: the one chunk this package actually wants. It
// validates fmt along the way (see parseFMTChunk) and refuses any
// chunk it is not already willing to skip (see wavSkippableChunkIDs),
// so a crafted file cannot hide a second data chunk, or anything
// else, behind a chunk kind this package has no reason to ever see.
func findDataChunk(f io.Reader, wantSampleRate int) (pcmFormat, int64, error) {
	scanner := &wavChunkScanner{}
	for {
		header, err := readChunkHeader(f)
		if err != nil {
			return pcmFormat{}, 0, err
		}
		dataLen, done, err := scanner.handle(f, header, wantSampleRate)
		if err != nil {
			return pcmFormat{}, 0, err
		}
		if done {
			return scanner.format, dataLen, nil
		}
	}
}

// wavChunkHeader is one chunk's own 4-byte id and declared size, as
// read by readChunkHeader.
type wavChunkHeader struct {
	id   string
	size int64
}

// readChunkHeader reads one chunk's own 8-byte id+size header.
func readChunkHeader(f io.Reader) (wavChunkHeader, error) {
	id := make([]byte, 4)
	if _, err := io.ReadFull(f, id); err != nil {
		return wavChunkHeader{}, fmt.Errorf("%w: reading chunk id: %w", ErrFailed, err)
	}
	size, err := readUint32LE(f)
	if err != nil {
		return wavChunkHeader{}, fmt.Errorf("%w: reading chunk size: %w", ErrFailed, err)
	}
	return wavChunkHeader{id: string(id), size: int64(size)}, nil
}

// parseFMTChunk reads exactly size bytes of a fmt chunk's own body
// (plus its pad byte, if size is odd) and validates the handful of
// fields a canonical PCM header needs: PCM format, one or two
// channels, exactly wantSampleRate, 16-bit samples. Anything past
// the first 16 bytes - a cbSize field some encoders add - is read
// and discarded along with the rest of the declared body, never
// inspected.
func parseFMTChunk(f io.Reader, size int64, wantSampleRate int) (pcmFormat, error) {
	if size < 16 {
		return pcmFormat{}, fmt.Errorf("%w: fmt chunk is %d bytes, want at least 16", ErrFailed, size)
	}

	body := make([]byte, size)
	if _, err := io.ReadFull(f, body); err != nil {
		return pcmFormat{}, fmt.Errorf("%w: reading fmt chunk: %w", ErrFailed, err)
	}
	if err := skipPad(f, size); err != nil {
		return pcmFormat{}, err
	}

	tag := binary.LittleEndian.Uint16(body[0:2])
	channels := binary.LittleEndian.Uint16(body[2:4])
	sampleRate := binary.LittleEndian.Uint32(body[4:8])
	bits := binary.LittleEndian.Uint16(body[14:16])

	return checkFMTFields(tag, channels, sampleRate, bits, wantSampleRate)
}

// checkFMTFields is parseFMTChunk's own field-by-field validation,
// split out so parseFMTChunk's own length stays short.
func checkFMTFields(tag, channels uint16, sampleRate uint32, bits uint16, wantSampleRate int) (pcmFormat, error) {
	switch {
	case tag != pcmFormatTag:
		return pcmFormat{}, fmt.Errorf("%w: fmt tag %d, want PCM", ErrFailed, tag)
	case channels != 1 && channels != 2:
		return pcmFormat{}, fmt.Errorf("%w: %d channels, want 1 or 2", ErrFailed, channels)
	case sampleRate != uint32(wantSampleRate):
		return pcmFormat{}, fmt.Errorf("%w: sample rate %d, want %d", ErrFailed, sampleRate, wantSampleRate)
	case bits != bitsPerSamplePCM16:
		return pcmFormat{}, fmt.Errorf("%w: %d bits per sample, want 16", ErrFailed, bits)
	default:
		return pcmFormat{channels: channels, sampleRate: sampleRate}, nil
	}
}

// readMagic reads exactly len(want) bytes from f and confirms they
// equal want: the only check this package makes on a RIFF/WAVE magic
// string.
func readMagic(f io.Reader, want string) error {
	got := make([]byte, len(want))
	if _, err := io.ReadFull(f, got); err != nil {
		return fmt.Errorf("%w: reading %q: %w", ErrFailed, want, err)
	}
	if string(got) != want {
		return fmt.Errorf("%w: expected %q, got %q", ErrFailed, want, got)
	}
	return nil
}

// readUint32LE reads one little-endian uint32 from f.
func readUint32LE(f io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

// skipPad consumes the one padding byte a chunk's own odd-sized body
// is followed by, so the next read starts aligned on the next chunk
// header; RIFF chunks are always word-aligned even though their own
// declared size field never counts this byte.
func skipPad(f io.Reader, size int64) error {
	if size%2 == 0 {
		return nil
	}
	var pad [1]byte
	if _, err := io.ReadFull(f, pad[:]); err != nil {
		return fmt.Errorf("%w: reading chunk padding: %w", ErrFailed, err)
	}
	return nil
}

// skipChunk discards size bytes (plus padding) of a chunk this
// package recognises as harmless but has no use for.
func skipChunk(f io.Reader, size int64) error {
	if _, err := io.CopyN(io.Discard, f, size); err != nil {
		return fmt.Errorf("%w: skipping chunk: %w", ErrFailed, err)
	}
	return skipPad(f, size)
}

// wavHeader builds the canonical 44-byte PCM header for format and
// dataLen.
func wavHeader(format pcmFormat, dataLen int64) []byte {
	const riffOverhead = wavHeaderSize - 8 // everything after the RIFF chunk's own 4-byte tag and 4-byte size field

	byteRate := format.sampleRate * uint32(format.channels) * bytesPerSample16
	blockAlign := format.channels * bytesPerSample16

	h := make([]byte, wavHeaderSize)
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], uint32(riffOverhead+dataLen))
	copy(h[8:12], "WAVE")
	copy(h[12:16], "fmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16)
	binary.LittleEndian.PutUint16(h[20:22], pcmFormatTag)
	binary.LittleEndian.PutUint16(h[22:24], format.channels)
	binary.LittleEndian.PutUint32(h[24:28], format.sampleRate)
	binary.LittleEndian.PutUint32(h[28:32], byteRate)
	binary.LittleEndian.PutUint16(h[32:34], blockAlign)
	binary.LittleEndian.PutUint16(h[34:36], bitsPerSamplePCM16)
	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], uint32(dataLen))
	return h
}

// writeWAV creates outPath with a canonical PCM header for format
// and dataLen, then copies exactly dataLen bytes read from data.
func writeWAV(outPath string, format pcmFormat, data io.Reader, dataLen int64) error {
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%w: creating output: %w", ErrFailed, err)
	}

	writeErr := writeWAVBody(out, format, data, dataLen)
	closeErr := out.Close()
	switch {
	case writeErr != nil:
		return writeErr
	case closeErr != nil:
		return fmt.Errorf("%w: closing output: %w", ErrFailed, closeErr)
	default:
		return nil
	}
}

// writeWAVBody writes the header and sample data to out, leaving
// out's own Close to writeWAV.
func writeWAVBody(out io.Writer, format pcmFormat, data io.Reader, dataLen int64) error {
	if _, err := out.Write(wavHeader(format, dataLen)); err != nil {
		return fmt.Errorf("%w: writing header: %w", ErrFailed, err)
	}
	if _, err := io.CopyN(out, data, dataLen); err != nil {
		return fmt.Errorf("%w: copying samples: %w", ErrFailed, err)
	}
	return nil
}
