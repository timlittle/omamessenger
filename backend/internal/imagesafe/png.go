package imagesafe

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	_ "image/png" // registers the PNG decoder Sanitize's own confirmation decode uses
)

// pngMaxChunks bounds how many chunks sanitizePNG will walk, so a file
// built from many tiny chunks cannot make the walk itself expensive;
// no real encoder produces anywhere near this many.
const pngMaxChunks = 10_000

// pngKeptChunks are the chunk types sanitizePNG copies through
// unchanged: everything a decoder needs to produce the same pixels
// (IHDR, PLTE, IDAT, IEND, tRNS), the handful of rendering-affecting
// ancillary chunks that carry no text or arbitrary payload (gAMA,
// cHRM, sRGB, pHYs), and APNG's own animation chunks (acTL, fcTL,
// fdAT). Every other chunk - zTXt, iTXt, tEXt and iCCP above all,
// since those are exactly the zlib-compressed payloads Qt's reader
// still inflates - is dropped.
var pngKeptChunks = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true, "tRNS": true,
	"gAMA": true, "cHRM": true, "sRGB": true, "pHYs": true,
	"acTL": true, "fcTL": true, "fdAT": true,
}

// sanitizePNG rebuilds data's chunk stream keeping only pngKeptChunks,
// after checking the dimensions IHDR declares against profile and
// every chunk's own length and CRC. ok is false for anything
// structurally wrong: a short header, a chunk that overruns the
// buffer, a CRC that does not match, or dimensions over profile's
// caps.
func sanitizePNG(data []byte, profile Profile) ([]byte, bool) {
	width, height, ok := pngDimensions(data)
	if !ok || !withinCaps(width, height, profile) {
		return nil, false
	}

	var out bytes.Buffer
	out.Write(pngSignature)

	pos := len(pngSignature)
	for i := 0; i < pngMaxChunks && pos < len(data); i++ {
		kind, body, next, ok := readPNGChunk(data, pos)
		if !ok {
			return nil, false
		}
		if pngKeptChunks[kind] {
			writePNGChunk(&out, kind, body)
		}

		pos = next
		if kind == "IEND" {
			return finishPNG(out.Bytes(), width, height)
		}
	}

	// The walk ran out of either bytes or its chunk budget without ever
	// seeing IEND: not a complete PNG, so nothing is trusted.
	return nil, false
}

// pngDimensions reads width and height straight out of data's IHDR
// chunk, the first chunk every valid PNG has, without touching
// anything compressed.
func pngDimensions(data []byte) (width, height int64, ok bool) {
	kind, body, _, ok := readPNGChunk(data, len(pngSignature))
	if !ok || kind != "IHDR" || len(body) < 8 {
		return 0, 0, false
	}

	return int64(binary.BigEndian.Uint32(body[0:4])), int64(binary.BigEndian.Uint32(body[4:8])), true
}

// readPNGChunk reads the one chunk starting at pos: its four-letter
// type, its body and the position right after it. ok is false when
// the length, type or CRC do not fit what remains of data.
func readPNGChunk(data []byte, pos int) (kind string, body []byte, next int, ok bool) {
	if pos+8 > len(data) {
		return "", nil, 0, false
	}

	length := binary.BigEndian.Uint32(data[pos : pos+4])
	kind = string(data[pos+4 : pos+8])
	bodyStart := pos + 8
	bodyEnd := bodyStart + int(length)
	if length > uint32(len(data)) || bodyEnd+4 > len(data) {
		return "", nil, 0, false
	}

	body = data[bodyStart:bodyEnd]
	wantCRC := binary.BigEndian.Uint32(data[bodyEnd : bodyEnd+4])
	if crc32.ChecksumIEEE(data[pos+4:bodyEnd]) != wantCRC {
		return "", nil, 0, false
	}

	return kind, body, bodyEnd + 4, true
}

// writePNGChunk appends one chunk - length, type, body and a freshly
// computed CRC - to out.
func writePNGChunk(out *bytes.Buffer, kind string, body []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(body)))
	out.Write(length[:])
	out.WriteString(kind)
	out.Write(body)

	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(append([]byte(kind), body...)))
	out.Write(crc[:])
}

// finishPNG confirms out still decodes, and to the same dimensions
// pngDimensions found, before trusting it as sanitizePNG's result:
// the chunk walk above never ran a real decoder, so this is the one
// check that the stripped stream is not just well-formed but actually
// renders.
func finishPNG(out []byte, width, height int64) ([]byte, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || int64(cfg.Width) != width || int64(cfg.Height) != height {
		return nil, false
	}

	return out, true
}
