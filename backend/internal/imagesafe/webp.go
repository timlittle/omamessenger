package imagesafe

import (
	"bytes"
	"encoding/binary"
)

// webpMaxFrames bounds how many ANMF frames sanitizeWebP will walk in
// an animated WebP, the same guard gifMaxFrames gives a GIF.
const webpMaxFrames = 1024

// webpVP8XFlagsICC, webpVP8XFlagsExif and webpVP8XFlagsXMP are the
// VP8X flags byte's bits that say an ICCP, EXIF or XMP chunk follows;
// cleared once those chunks are dropped, so the rewritten header never
// promises a chunk that is no longer there.
const (
	webpVP8XFlagsICC  = 0x20
	webpVP8XFlagsExif = 0x08
	webpVP8XFlagsXMP  = 0x04
)

// sanitizeWebP rebuilds data's RIFF chunk list, dropping ICCP, EXIF
// and XMP metadata chunks and adjusting VP8X's own flags to match,
// after checking the canvas dimensions - read from VP8X when present,
// otherwise straight from the VP8 or VP8L bitstream header - and every
// animation frame's own dimensions against profile. Nothing here
// decodes the actual image data: Go has no WebP decoder to confirm
// against, so correctness rests entirely on the chunk and bitstream
// headers being well-formed, which is checked at every step. ok is
// false for anything structurally wrong or over profile's caps.
func sanitizeWebP(data []byte, profile Profile) ([]byte, bool) {
	var body bytes.Buffer
	canvasWidth, canvasHeight := int64(-1), int64(-1)
	frames := 0

	for pos := 12; pos < len(data); {
		fourCC, payload, next, ok := readRIFFChunk(data, pos)
		if !ok {
			return nil, false
		}
		pos = next

		kept, w, h, ok := webpChunk(fourCC, payload, profile, &frames)
		if !ok {
			return nil, false
		}
		if w >= 0 && canvasWidth < 0 {
			canvasWidth, canvasHeight = w, h
		}
		if kept != nil {
			writeRIFFChunk(&body, fourCC, kept)
		}
	}

	if canvasWidth < 0 || !withinCaps(canvasWidth, canvasHeight, profile) {
		return nil, false
	}

	return assembleWebP(body.Bytes()), true
}

// webpChunk decides what one top-level chunk contributes: the bytes
// to keep (nil to drop the chunk entirely) and, when this chunk is
// the one that carries the image's own dimensions, what they are
// (width -1 otherwise). frames is incremented, and capped, for every
// ANMF chunk walked.
func webpChunk(fourCC string, payload []byte, profile Profile, frames *int) (kept []byte, width, height int64, ok bool) {
	switch fourCC {
	case "ICCP", "EXIF", "XMP ":
		return nil, -1, -1, true
	case "VP8X":
		w, h, ok := vp8xDimensions(payload)
		return adjustVP8XFlags(payload), w, h, ok
	case "VP8 ":
		w, h, ok := vp8Dimensions(payload)
		return payload, w, h, ok
	case "VP8L":
		w, h, ok := vp8lDimensions(payload)
		return payload, w, h, ok
	case "ANMF":
		*frames++
		if *frames > webpMaxFrames {
			return nil, -1, -1, false
		}
		frame, ok := sanitizeFrame(payload, profile)
		return frame, -1, -1, ok
	case "ANIM", "ALPH":
		return payload, -1, -1, true
	default:
		return nil, -1, -1, true
	}
}

// sanitizeFrame checks one ANMF frame's own declared size against
// profile and drops any metadata chunk nested inside it.
func sanitizeFrame(payload []byte, profile Profile) ([]byte, bool) {
	if len(payload) < 16 {
		return nil, false
	}

	raw := int64(payload[6]) | int64(payload[7])<<8 | int64(payload[8])<<16
	width := raw + 1
	raw = int64(payload[9]) | int64(payload[10])<<8 | int64(payload[11])<<16
	height := raw + 1
	if !withinCaps(width, height, profile) {
		return nil, false
	}

	var body bytes.Buffer
	for pos := 16; pos < len(payload); {
		fourCC, chunkPayload, next, ok := readRIFFChunk(payload, pos)
		if !ok {
			return nil, false
		}
		if fourCC == "ALPH" || fourCC == "VP8 " || fourCC == "VP8L" {
			writeRIFFChunk(&body, fourCC, chunkPayload)
		}
		pos = next
	}

	return append(append([]byte{}, payload[0:16]...), body.Bytes()...), true
}

// vp8xDimensions reads a VP8X chunk's canvas width and height, each a
// 24-bit little-endian value one less than the real size.
func vp8xDimensions(payload []byte) (width, height int64, ok bool) {
	if len(payload) < 10 {
		return 0, 0, false
	}

	w := int64(payload[4]) | int64(payload[5])<<8 | int64(payload[6])<<16
	h := int64(payload[7]) | int64(payload[8])<<8 | int64(payload[9])<<16

	return w + 1, h + 1, true
}

// adjustVP8XFlags clears the ICC, EXIF and XMP flag bits of a copy of
// a VP8X chunk's payload, so the chunk no longer promises metadata
// that was just dropped from around it.
func adjustVP8XFlags(payload []byte) []byte {
	out := append([]byte{}, payload...)
	if len(out) > 0 {
		out[0] &^= webpVP8XFlagsICC | webpVP8XFlagsExif | webpVP8XFlagsXMP
	}

	return out
}

// vp8Dimensions reads a lossy VP8 bitstream's width and height from
// its key frame header: a three-byte frame tag, a three-byte start
// code, then two 14-bit, little-endian size fields.
func vp8Dimensions(payload []byte) (width, height int64, ok bool) {
	if len(payload) < 10 || payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
		return 0, 0, false
	}

	w := int64(payload[6]) | int64(payload[7])<<8
	h := int64(payload[8]) | int64(payload[9])<<8

	return w & 0x3FFF, h & 0x3FFF, true
}

// vp8lDimensions reads a lossless VP8L bitstream's width and height,
// bit-packed into a 32-bit little-endian field right after its
// one-byte signature.
func vp8lDimensions(payload []byte) (width, height int64, ok bool) {
	if len(payload) < 5 || payload[0] != 0x2f {
		return 0, 0, false
	}

	bits := uint32(payload[1]) | uint32(payload[2])<<8 | uint32(payload[3])<<16 | uint32(payload[4])<<24

	return int64(bits&0x3FFF) + 1, int64((bits>>14)&0x3FFF) + 1, true
}

// readRIFFChunk reads the one chunk starting at pos: its four-letter
// tag, its payload and the position right after it, including the
// single pad byte a chunk of odd length carries so the next chunk
// always starts on an even offset.
func readRIFFChunk(data []byte, pos int) (fourCC string, payload []byte, next int, ok bool) {
	if pos+8 > len(data) {
		return "", nil, 0, false
	}

	fourCC = string(data[pos : pos+4])
	size := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
	start := pos + 8
	end := start + int(size)
	if size > uint32(len(data)) || end > len(data) {
		return "", nil, 0, false
	}

	next = end
	if size%2 == 1 && end < len(data) {
		next++
	}

	return fourCC, data[start:end], next, true
}

// writeRIFFChunk appends one chunk - its tag, little-endian size,
// payload and, when the payload is odd-length, a zero pad byte - to out.
func writeRIFFChunk(out *bytes.Buffer, fourCC string, payload []byte) {
	out.WriteString(fourCC)

	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out.Write(size[:])
	out.Write(payload)

	if len(payload)%2 == 1 {
		out.WriteByte(0)
	}
}

// assembleWebP wraps body - the chunks sanitizeWebP kept, concatenated
// - in a fresh RIFF/WEBP header sized to match.
func assembleWebP(body []byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")

	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(4+len(body)))
	out.Write(size[:])
	out.WriteString("WEBP")
	out.Write(body)

	return out.Bytes()
}
